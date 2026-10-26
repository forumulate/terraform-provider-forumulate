package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func planWebhook() webhookModel {
	return webhookModel{
		ID: types.StringUnknown(), URL: S("https://hooks.example.com/f"), Events: setOfStrings([]string{"page.published"}),
		Format: types.StringUnknown(), Kind: types.StringUnknown(), BodyEncoding: types.StringUnknown(),
		SignatureVersion: types.StringUnknown(), Active: types.BoolUnknown(), Secret: types.StringUnknown(),
		State: types.StringUnknown(), CreatedAt: types.StringUnknown(),
		Metadata:    types.MapNull(types.StringType),
		CategoryIDs: types.SetNull(types.StringType), TagIDs: types.SetNull(types.StringType), GroupIDs: types.SetNull(types.StringType),
		ContentTypes: types.SetNull(types.StringType), Locales: types.SetNull(types.StringType), AlertContacts: types.SetNull(types.StringType),
	}
}

func webhookRow(active bool) map[string]any {
	return map[string]any{
		"id": "wh-1", "url": "https://hooks.example.com/f", "events": []any{"page.published"}, "active": active, "format": "json",
		"kind": "webhook", "bodyEncoding": "json", "signatureVersion": "v1", "state": "healthy", "createdAt": "2027-01-01T00:00:00Z",
		"spaceId": nil, "libraryId": nil, "categoryIds": nil, "tagIds": nil, "groupIds": nil, "contentTypes": nil, "locales": nil,
		"alert_contacts": []any{}, "metadata": map[string]any{},
	}
}

func TestWebhookCRUD(t *testing.T) {
	f := newFakeAPI(t)
	active := true
	var contacts []any = []any{}
	var catIDs any
	f.on("POST /orgs/org1/webhooks", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"webhook": webhookRow(true), "data": webhookRow(true), "secret": "whsec_once", "secretShown": true}
	})
	f.on("GET /orgs/org1/webhooks", 200, func(*http.Request, map[string]any) any {
		row := webhookRow(active)
		row["alert_contacts"] = contacts
		row["categoryIds"] = catIDs
		return map[string]any{"data": []any{row}}
	})
	f.on("PATCH /orgs/org1/webhooks/wh-1", 200, func(r *http.Request, b map[string]any) any {
		if v, ok := b["active"].(bool); ok {
			active = v
		}
		if v, has := b["categoryIds"]; has {
			catIDs = v
		}
		if v, ok := b["alertContacts"].([]any); ok {
			contacts = v
		}
		row := webhookRow(active)
		row["alert_contacts"] = contacts
		row["categoryIds"] = catIDs
		return map[string]any{"webhook": row}
	})
	f.on("DELETE /orgs/org1/webhooks/wh-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })
	d := newDriver[webhookModel](t, f, NewWebhookResource)

	plan := planWebhook()
	plan.AlertContacts = setOfStrings([]string{"ops@example.com"})
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if state.Secret.ValueString() != "whsec_once" {
		t.Fatalf("the signing secret is shown once, at creation, and must be kept: %q", state.Secret.ValueString())
	}
	create := f.find("POST", "/orgs/org1/webhooks")[0].Body
	if _, has := create["alertContacts"]; has {
		t.Fatalf("alertContacts is not on the create body: %v", create)
	}
	if patch := f.find("PATCH", "/orgs/org1/webhooks/wh-1")[0].Body; patch["alertContacts"] == nil {
		t.Fatalf("alert contacts go in a follow-up PATCH: %v", patch)
	}
	if len(state.AlertContacts.Elements()) != 1 || !state.CategoryIDs.IsNull() {
		t.Fatalf("state = %+v", state)
	}

	// Pausing it outside Terraform is drift on `active`; the secret survives reads.
	active = false
	read, _, diags := d.read(state)
	mustNoErr(t, diags)
	if read.Active.ValueBool() || read.Secret.ValueString() != "whsec_once" {
		t.Fatalf("read = active %v secret %q", read.Active.ValueBool(), read.Secret.ValueString())
	}
	active = true

	// Updating keeps the secret, narrows a scope, and clears a removed one.
	up := state
	up.Events = setOfStrings([]string{"page.published", "page.deleted"})
	up.CategoryIDs = setOfStrings([]string{"2f2f2f2f-0000-4000-8000-000000000000"})
	f.reset()
	state2, diags := d.update(up, state)
	mustNoErr(t, diags)
	pb := f.find("PATCH", "/orgs/org1/webhooks/wh-1")[0].Body
	if pb["categoryIds"] == nil || len(pb["events"].([]any)) != 2 {
		t.Fatalf("patch = %v", pb)
	}
	if state2.Secret.ValueString() != "whsec_once" {
		t.Fatal("the secret must survive an update")
	}
	back := state2
	back.CategoryIDs = types.SetNull(types.StringType)
	f.reset()
	_, diags = d.update(back, state2)
	mustNoErr(t, diags)
	if v, has := f.find("PATCH", "/orgs/org1/webhooks/wh-1")[0].Body["categoryIds"]; !has || v != nil {
		t.Fatal("a removed scope must be sent as null")
	}

	mustNoErr(t, d.delete(state))
	imp, _, diags := d.importState("wh-1")
	mustNoErr(t, diags)
	if imp.Secret.ValueString() != "" || !imp.Secret.IsNull() {
		t.Fatalf("an imported webhook has no secret: %+v", imp.Secret)
	}
}

func TestWebhookValidation(t *testing.T) {
	d := newDriver[webhookModel](t, newFakeAPI(t), NewWebhookResource)
	if errs := d.attrErrors("events", setOfStrings([]string{"page.published"})); len(errs) != 0 {
		t.Errorf("a real event: %v", errs)
	}
	if errs := d.attrErrors("events", setOfStrings([]string{"page.exploded"})); len(errs) == 0 {
		t.Error("an unknown event must be refused at plan time")
	}
	if errs := d.attrErrors("events", setOfStrings(nil)); len(errs) == 0 {
		t.Error("at least one event")
	}
	if errs := d.attrErrors("url", S("ftp://x.test")); len(errs) == 0 {
		t.Error("url must be http(s)")
	}
	if errs := d.attrErrors("alert_contacts", setOfStrings([]string{"not-an-email"})); len(errs) == 0 {
		t.Error("alert contacts are emails")
	}
	many := make([]string, 11)
	for i := range many {
		many[i] = "a@example.com"
	}
	if errs := d.attrErrors("alert_contacts", setOfStrings(many)); len(errs) == 0 {
		t.Error("at most 10 alert contacts")
	}
	if errs := d.attrErrors("format", S("teams")); len(errs) == 0 {
		t.Error("format enum")
	}
	both := planWebhook()
	both.SpaceID = S("2f2f2f2f-0000-4000-8000-000000000000")
	both.LibraryID = S("3f2f2f2f-0000-4000-8000-000000000000")
	mustErr(t, d.validate(both), "not both")
	if len(webhookEvents) < 300 {
		t.Fatalf("the generated event list looks truncated: %d", len(webhookEvents))
	}
}
