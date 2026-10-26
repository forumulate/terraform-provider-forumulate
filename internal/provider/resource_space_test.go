package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// planSpace is a plan as Terraform builds it: configured values set, every
// computed attribute the user did not set unknown.
func planSpace() spaceModel {
	return spaceModel{
		ID: types.StringUnknown(), Slug: S("docs"), Name: S("Docs"), Type: S("DOCS"),
		Visibility: types.StringUnknown(), CreatedAt: types.StringUnknown(),
		Shape: types.ObjectNull(shapeTypes), Metadata: types.MapNull(types.StringType),
	}
}

func spaceJSON(name, vis string, settings map[string]any) map[string]any {
	return map[string]any{
		"id": "sp-1", "slug": "docs", "name": name, "type": "DOCS", "visibility": vis,
		"createdAt": "2027-01-02T03:04:05Z", "settings": settings,
	}
}

func TestSpaceCRUD(t *testing.T) {
	f := newFakeAPI(t)
	settings := map[string]any{"translations": map[string]any{"es": map[string]any{"name": "Documentación"}}, "analytics": map[string]any{"id": "x"}}
	f.on("POST /orgs/org1/spaces", 201, func(r *http.Request, b map[string]any) any {
		s := spaceJSON("Docs", "PRIVATE", map[string]any{})
		return map[string]any{"space": s, "data": s}
	})
	f.on("PATCH /orgs/org1/spaces/sp-1", 200, func(r *http.Request, b map[string]any) any {
		name, _ := b["name"].(string)
		return map[string]any{"space": spaceJSON(name, "PUBLIC", settings)}
	})
	f.on("GET /orgs/org1/spaces/sp-1", 200, func(r *http.Request, b map[string]any) any {
		return map[string]any{"space": spaceJSON("Docs", "PUBLIC", settings)}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })
	f.on("DELETE /orgs/org1/spaces/sp-1/destroy", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })

	d := newDriver[spaceModel](t, f, NewSpaceResource)

	plan := planSpace()
	plan.Visibility = S("PUBLIC")
	plan.Preset = S("standard")
	plan.TranslationsJSON = S(`{"es":{"name":"Documentación"}}`)
	plan.SettingsJSON = S(`{"analytics":{"id":"x"}}`)
	state, diags := d.create(plan)
	mustNoErr(t, diags)

	post := f.find("POST", "/orgs/org1/spaces")[0]
	if post.Body["slug"] != "docs" || post.Body["type"] != "DOCS" || post.Body["visibility"] != "PUBLIC" || post.Body["preset"] != "standard" {
		t.Fatalf("create body = %v", post.Body)
	}
	if _, has := post.Body["settings"]; has {
		t.Fatalf("settings must go in a PATCH, the create body does not take them: %v", post.Body)
	}
	if post.Header.Get("Forumulate-Version") != "2027-02-01" || post.Header.Get("Authorization") != "Bearer fp_test" || post.Header.Get("Idempotency-Key") == "" {
		t.Fatalf("headers = %v", post.Header)
	}
	patch := f.find("PATCH", "/orgs/org1/spaces/sp-1")[0]
	if patch.Body["translations"] == nil || patch.Body["settings"] == nil {
		t.Fatalf("patch body = %v", patch.Body)
	}
	if state.ID.ValueString() != "sp-1" || state.CreatedAt.ValueString() == "" || state.Visibility.ValueString() != "PUBLIC" {
		t.Fatalf("state = %+v", state)
	}
	// The configured spelling is kept while it means what the API holds.
	if state.SettingsJSON.ValueString() != `{"analytics":{"id":"x"}}` {
		t.Fatalf("settings_json = %s", state.SettingsJSON.ValueString())
	}

	// Read: the API changed a declared setting outside Terraform -> drift shows.
	settings["analytics"] = map[string]any{"id": "changed"}
	read, gone, diags := d.read(state)
	mustNoErr(t, diags)
	if gone || read.SettingsJSON.ValueString() != `{"analytics":{"id":"changed"}}` {
		t.Fatalf("drift not detected: %s", read.SettingsJSON.ValueString())
	}
	settings["analytics"] = map[string]any{"id": "x"}

	// Update: rename, drop the translations, remove nothing else.
	up := state
	up.Name = S("Documentation")
	up.TranslationsJSON = types.StringNull()
	f.reset()
	newState, diags := d.update(up, state)
	mustNoErr(t, diags)
	pb := f.last().Body
	if pb["name"] != "Documentation" || pb["translations"] != nil {
		t.Fatalf("update body = %v", pb)
	}
	if _, has := pb["translations"]; !has {
		t.Fatalf("a removed translations block must be sent as null to clear it: %v", pb)
	}
	if _, has := pb["settings"]; has {
		t.Fatalf("settings were unchanged, must not be re-sent: %v", pb)
	}
	if newState.Name.ValueString() != "Documentation" {
		t.Fatalf("name = %s", newState.Name.ValueString())
	}

	// Delete trashes; destroy_on_delete also destroys, confirming with the slug.
	f.reset()
	mustNoErr(t, d.delete(newState))
	if len(f.find("DELETE", "/orgs/org1/spaces/sp-1/destroy")) != 0 {
		t.Fatal("a plain delete must only trash")
	}
	newState.DestroyOnDelete = types.BoolValue(true)
	mustNoErr(t, d.delete(newState))
	if q := f.find("DELETE", "/orgs/org1/spaces/sp-1/destroy"); len(q) != 1 || q[0].Query != "confirm=docs" {
		t.Fatalf("destroy calls = %+v", q)
	}
}

func TestSpaceReadGoneAndImportAndErrors(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET /orgs/org1/spaces/gone", 404, func(*http.Request, map[string]any) any {
		return fail(404, "space.not_found", "No such space.", "spaceId")
	})
	f.on("GET /orgs/org1/spaces/docs", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"space": spaceJSON("Docs", "PUBLIC", map[string]any{})}
	})
	f.on("POST /orgs/org1/spaces", 422, func(*http.Request, map[string]any) any {
		e := fail(422, "validation.failed", "Validation failed.", "settings")
		e.env["hint"] = "Use the theme door."
		e.env["details"] = map[string]any{"settings": []string{"theme has its own route"}}
		return e
	})
	d := newDriver[spaceModel](t, f, NewSpaceResource)

	gone := planSpace()
	gone.ID = S("gone")
	_, removed, diags := d.read(gone)
	mustNoErr(t, diags)
	if !removed {
		t.Fatal("a 404 on read must remove the resource from state")
	}

	imp, removed, diags := d.importState("docs")
	mustNoErr(t, diags)
	if removed || imp.Slug.ValueString() != "docs" || imp.Name.ValueString() != "Docs" || imp.ID.ValueString() != "sp-1" {
		t.Fatalf("imported = %+v", imp)
	}

	bad := planSpace()
	bad.SettingsJSON = S(`{"theme":{}}`)
	_, diags = d.create(bad)
	mustErr(t, diags, "Hint: Use the theme door.")
	mustErr(t, diags, "req_test")
	if p := diags.Errors()[0]; p.Summary() != "Forumulate API: creating space" {
		t.Fatalf("summary = %q", p.Summary())
	}
}

func TestSpaceValidation(t *testing.T) {
	d := newDriver[spaceModel](t, newFakeAPI(t), NewSpaceResource)
	for name, c := range map[string]struct {
		attr string
		v    types.String
		ok   bool
	}{
		"slug ok":          {"slug", S("my-docs"), true},
		"slug uppercase":   {"slug", S("My-Docs"), false},
		"slug too short":   {"slug", S("a"), false},
		"slug double dash": {"slug", S("a--b"), false},
		"slug too long":    {"slug", S("a1234567890123456789012345678901234567890123456789"), false},
		"type ok":          {"type", S("FORUM"), true},
		"type unknown":     {"type", S("WIKI"), false},
		"visibility bad":   {"visibility", S("HIDDEN"), false},
		"name empty":       {"name", S(""), false},
		"settings array":   {"settings_json", S(`[1]`), false},
		"settings bad":     {"settings_json", S(`{`), false},
		"settings ok":      {"settings_json", S(`{"a":1}`), true},
		"ext ref space":    {"external_ref", S("has space"), false},
	} {
		errs := d.attrErrors(c.attr, c.v)
		if (len(errs) == 0) != c.ok {
			t.Errorf("%s: errors = %v", name, errs)
		}
	}
	if errs := d.attrErrors("metadata", types.MapValueMust(types.StringType, map[string]attr.Value{"bad key!": S("x")})); len(errs) == 0 {
		t.Error("a metadata key with a bang must be refused")
	}
}
