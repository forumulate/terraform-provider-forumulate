package provider

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func planSite() siteModel {
	return siteModel{
		ID: types.StringUnknown(), Slug: S("main"), Name: S("Main"), CreatedAt: types.StringUnknown(),
		SpaceIDs: types.SetNull(types.StringType), PageMounts: types.ListNull(types.ObjectType{AttrTypes: pageMountTypes}),
		Metadata: types.MapNull(types.StringType),
	}
}

func TestSiteCRUD(t *testing.T) {
	f := newFakeAPI(t)
	site := map[string]any{
		"id": "st-1", "slug": "main", "name": "Main", "createdAt": "2027-01-01T00:00:00Z", "spaceIds": []any{"a"},
		"config": map[string]any{"copyright": "Acme", "mobileNav": "bar", "pageMounts": []any{}},
	}
	f.on("POST /orgs/org1/sites", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"site": site}
	})
	f.on("GET /orgs/org1/sites/st-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"site": site} })
	f.on("PATCH /orgs/org1/sites/st-1", 200, func(r *http.Request, b map[string]any) any {
		cfg, _ := site["config"].(map[string]any)
		if c, ok := b["config"].(map[string]any); ok {
			for k, v := range c {
				if v == nil {
					delete(cfg, k)
				} else {
					cfg[k] = v
				}
			}
		}
		if ids, ok := b["spaceIds"].([]any); ok {
			site["spaceIds"] = ids
		}
		site["name"] = b["name"]
		return map[string]any{"site": site, "contrastWarnings": []any{map[string]any{"code": "contrast.low", "key": "cta", "message": "Low contrast on the CTA."}}}
	})
	f.on("DELETE /orgs/org1/sites/st-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })
	d := newDriver[siteModel](t, f, NewSiteResource)

	plan := planSite()
	plan.ConfigJSON = S(`{"copyright":"Acme","footerDescription":"Hi"}`)
	plan.SpaceIDs = setOfStrings([]string{"a"})
	mount, _ := types.ObjectValue(pageMountTypes, map[string]attr.Value{"path": S("/pricing"), "space_id": S("a"), "page": S("pricing")})
	plan.PageMounts, _ = types.ListValue(types.ObjectType{AttrTypes: pageMountTypes}, []attr.Value{mount})
	site["config"].(map[string]any)["pageMounts"] = []any{map[string]any{"path": "/pricing", "spaceId": "a", "page": "pricing"}}
	site["config"].(map[string]any)["footerDescription"] = "Hi"

	state, diags := d.create(plan)
	mustNoErr(t, diags)
	body := f.find("POST", "/orgs/org1/sites")[0].Body
	cfg := body["config"].(map[string]any)
	if cfg["copyright"] != "Acme" || cfg["pageMounts"] == nil || body["slug"] != "main" {
		t.Fatalf("create body = %v", body)
	}
	mounts := cfg["pageMounts"].([]any)[0].(map[string]any)
	if mounts["path"] != "/pricing" || mounts["spaceId"] != "a" || mounts["page"] != "pricing" {
		t.Fatalf("mount body = %v", mounts)
	}
	if state.ConfigJSON.ValueString() != `{"copyright":"Acme","footerDescription":"Hi"}` {
		t.Fatalf("config_json = %s", state.ConfigJSON.ValueString())
	}
	if len(state.PageMounts.Elements()) != 1 {
		t.Fatalf("mounts = %v", state.PageMounts)
	}

	// Other config keys (mobileNav) are not drift; a changed declared key is.
	site["config"].(map[string]any)["copyright"] = "Other"
	read, _, diags := d.read(state)
	mustNoErr(t, diags)
	var got map[string]any
	_ = json.Unmarshal([]byte(read.ConfigJSON.ValueString()), &got)
	if got["copyright"] != "Other" || got["mobileNav"] != nil {
		t.Fatalf("drift read = %v", got)
	}
	site["config"].(map[string]any)["copyright"] = "Acme"

	// Update: drop footerDescription, leave the mounts: it is sent as null.
	up := state
	up.ConfigJSON = S(`{"copyright":"Acme"}`)
	up.Name = S("Main site")
	up.SpaceIDs = setOfStrings([]string{"a", "b"})
	f.reset()
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	if len(diags.Warnings()) != 1 {
		t.Fatalf("contrast warning not surfaced: %v", diags)
	}
	pb := f.find("PATCH", "/orgs/org1/sites/st-1")[0].Body
	pc := pb["config"].(map[string]any)
	if v, has := pc["footerDescription"]; !has || v != nil {
		t.Fatalf("removed config key must be null: %v", pc)
	}
	if ids := pb["spaceIds"].([]any); len(ids) != 2 {
		t.Fatalf("space ids = %v", pb["spaceIds"])
	}

	// Removing every mount clears the list.
	nm := state
	nm.PageMounts = types.ListNull(types.ObjectType{AttrTypes: pageMountTypes})
	f.reset()
	_, diags = d.update(nm, state)
	mustNoErr(t, diags)
	if pm := f.find("PATCH", "/orgs/org1/sites/st-1")[0].Body["config"].(map[string]any)["pageMounts"]; len(pm.([]any)) != 0 {
		t.Fatalf("pageMounts must be cleared with []: %v", pm)
	}

	mustNoErr(t, d.delete(state))
}

func TestSiteValidation(t *testing.T) {
	d := newDriver[siteModel](t, newFakeAPI(t), NewSiteResource)
	if errs := d.attrErrors("config_json", S(`{"cta":{"label":"x","href":"/"}}`)); len(errs) != 0 {
		t.Errorf("a real config key: %v", errs)
	}
	if errs := d.attrErrors("config_json", S(`{"ctaa":{}}`)); len(errs) == 0 {
		t.Error("an unknown config key must be refused")
	}
	if errs := d.attrErrors("config_json", S(`{"pageMounts":[]}`)); len(errs) == 0 {
		t.Error("page mounts have their own attribute")
	}
	if errs := d.attrErrors("slug", S("A")); len(errs) == 0 {
		t.Error("slug")
	}
	bad := planSite()
	m1, _ := types.ObjectValue(pageMountTypes, map[string]attr.Value{"path": S("/x"), "space_id": S("a"), "page": S("p")})
	bad.PageMounts, _ = types.ListValue(types.ObjectType{AttrTypes: pageMountTypes}, []attr.Value{m1, m1})
	mustErr(t, d.validate(bad), "Duplicate mount")
	mp, _ := types.ObjectValue(pageMountTypes, map[string]attr.Value{"path": S("pricing"), "space_id": S("a"), "page": S("p")})
	_ = mp
}
