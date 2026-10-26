package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSpaceConnectorCRUDAndToken(t *testing.T) {
	f := newFakeAPI(t)
	row := map[string]any{
		"id": "cn-1", "providerId": "intercom", "providerName": "Intercom", "embedEnabled": true, "knowledgeEnabled": true,
		"knowledgeScopes": []any{"search"}, "appearance": map[string]any{"position": "right", "colorScheme": "system"},
		"configuredFields": []any{"workspaceId"}, "missingFields": []any{"accessToken"}, "hasToken": false, "tokenPrefix": nil,
	}
	f.on("POST /orgs/org1/spaces/sp-1/connectors", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"connector": row, "data": row}
	})
	f.on("GET /orgs/org1/spaces/sp-1/connectors", 200, func(*http.Request, map[string]any) any { return map[string]any{"data": []any{row}} })
	f.on("PATCH /orgs/org1/spaces/sp-1/connectors/cn-1", 200, func(r *http.Request, b map[string]any) any {
		if v, ok := b["embedEnabled"].(bool); ok {
			row["embedEnabled"] = v
		}
		return map[string]any{"connector": row}
	})
	minted := 0
	f.on("POST /orgs/org1/spaces/sp-1/connectors/cn-1/token", 201, func(*http.Request, map[string]any) any {
		minted++
		row["hasToken"], row["tokenPrefix"] = true, "fpc_ab"
		return map[string]any{"token": "fpc_ab_secret", "tokenPrefix": "fpc_ab", "scopes": []any{"search"}}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1/connectors/cn-1/token", 200, func(*http.Request, map[string]any) any {
		row["hasToken"], row["tokenPrefix"] = false, nil
		return map[string]any{"revoked": true}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1/connectors/cn-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"deleted": true} })

	d := newDriver[connectorModel](t, f, NewConnectorResource)
	appAt := attrTypesOf(connectorAppearanceFields)
	app, _ := types.ObjectValue(appAt, map[string]attr.Value{"primary_color": types.StringNull(), "color_scheme": S("system"), "position": S("right"), "greeting": types.StringNull()})
	plan := connectorModel{
		ID: types.StringUnknown(), SpaceID: S("sp-1"), ProviderID: S("intercom"),
		Config:       types.MapValueMust(types.StringType, map[string]attr.Value{"workspaceId": S("abc")}),
		EmbedEnabled: types.BoolValue(true), KnowledgeEnabled: types.BoolValue(true), KnowledgeScopes: setOfStrings([]string{"search"}),
		Appearance: app, Metadata: types.MapNull(types.StringType), MintToken: types.BoolValue(true),
		Token: types.StringUnknown(), TokenPrefix: types.StringUnknown(), ProviderName: types.StringUnknown(),
		ConfiguredFields: types.SetUnknown(types.StringType), MissingFields: types.SetUnknown(types.StringType),
	}
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if len(diags.Warnings()) != 1 {
		t.Fatalf("an incomplete connector must warn: %v", diags)
	}
	body := f.find("POST", "/orgs/org1/spaces/sp-1/connectors")[0].Body
	if body["providerId"] != "intercom" || body["config"].(map[string]any)["workspaceId"] != "abc" || body["embedEnabled"] != true {
		t.Fatalf("body = %v", body)
	}
	if state.Token.ValueString() != "fpc_ab_secret" || state.TokenPrefix.ValueString() != "fpc_ab" || minted != 1 {
		t.Fatalf("token = %q prefix %q minted %d", state.Token.ValueString(), state.TokenPrefix.ValueString(), minted)
	}
	if got := state.MissingFields.Elements(); len(got) != 1 || got[0].(types.String).ValueString() != "accessToken" {
		t.Fatalf("missing = %v", got)
	}

	// Reads keep the token; a token revoked outside Terraform shows as a difference.
	rd, _, diags := d.read(state)
	mustNoErr(t, diags)
	if rd.Token.ValueString() != "fpc_ab_secret" || !rd.MintToken.ValueBool() {
		t.Fatalf("read = %+v", rd)
	}
	row["hasToken"], row["tokenPrefix"] = false, nil
	rd, _, diags = d.read(state)
	mustNoErr(t, diags)
	if rd.MintToken.ValueBool() {
		t.Fatal("a revoked token must read as mint_token = false so the next apply mints again")
	}
	row["hasToken"], row["tokenPrefix"] = true, "fpc_ab"

	// Rotating mints a new token; turning mint_token off revokes it.
	rot := state
	rot.TokenRotation = S("2")
	rot.Token, rot.TokenPrefix = types.StringUnknown(), types.StringUnknown()
	f.reset()
	st2, diags := d.update(rot, state)
	mustNoErr(t, diags)
	if len(f.find("POST", "/orgs/org1/spaces/sp-1/connectors/cn-1/token")) != 1 || st2.Token.ValueString() != "fpc_ab_secret" {
		t.Fatal("changing token_rotation must mint")
	}
	off := st2
	off.MintToken = types.BoolValue(false)
	off.Token, off.TokenPrefix = types.StringUnknown(), types.StringUnknown()
	f.reset()
	st3, diags := d.update(off, st2)
	mustNoErr(t, diags)
	if len(f.find("DELETE", "/orgs/org1/spaces/sp-1/connectors/cn-1/token")) != 1 || !st3.Token.IsNull() {
		t.Fatalf("mint_token = false must revoke: %+v", st3)
	}

	// The plan marks the token unknown exactly when it will change.
	resp := resource.ModifyPlanResponse{Plan: d.plan(rot)}
	rot2 := state
	rot2.TokenRotation = S("3")
	d.r.(resource.ResourceWithModifyPlan).ModifyPlan(d.ctx, resource.ModifyPlanRequest{Plan: d.plan(rot2), State: d.state(state)}, &resp)
	var planned connectorModel
	mustNoErr(t, resp.Plan.Get(d.ctx, &planned))
	if !planned.Token.IsUnknown() {
		t.Fatalf("a rotation must plan an unknown token, got %v", planned.Token)
	}

	mustNoErr(t, d.delete(state))

	// Validation.
	if errs := d.attrErrors("knowledge_scopes", setOfStrings([]string{"search", "everything"})); len(errs) == 0 {
		t.Error("scopes enum")
	}
	if errs := d.attrErrors("provider_id", S("x")); len(errs) == 0 {
		t.Error("provider id min 2")
	}
	bad := plan
	bad.MintToken = types.BoolValue(false)
	bad.TokenRotation = S("1")
	mustErr(t, d.validate(bad), "token_rotation needs mint_token")
	hex := rschema.StringAttribute{Validators: []validator.String{hexColor()}}
	if errs := runValidators(d.ctx, hex, "primary_color", S("red")); len(errs) == 0 {
		t.Error("a theme colour is six-digit hex")
	}
	if errs := runValidators(d.ctx, hex, "primary_color", S("0b835b")); len(errs) == 0 {
		t.Error("the leading # is required")
	}
	if errs := runValidators(d.ctx, hex, "primary_color", S("#0b835b")); len(errs) != 0 {
		t.Errorf("a valid colour: %v", errs)
	}
}
