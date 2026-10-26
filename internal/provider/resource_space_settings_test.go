package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func redirectNestedFrom() rschema.Attribute {
	return rschema.StringAttribute{Validators: []validator.String{redirectPath()}}
}

func emptySettings() spaceSettingsModel {
	return spaceSettingsModel{
		ID: types.StringUnknown(), SpaceID: S("sp-1"),
		Redirects: types.ListNull(types.ObjectType{AttrTypes: redirectTypes}),
	}
}

func TestSpaceSettingsCRUD(t *testing.T) {
	f := newFakeAPI(t)
	seo := map[string]any{"seoTitle": "Docs", "siteName": "Acme", "noindex": false}
	theme := map[string]any{"primaryColor": "#0b835b", "fontBody": "Inter"}
	blog := map[string]any{"whoCanPost": "anyone", "commentOrder": "newest"}
	preset := map[string]any{"preset": "wide", "piecePreset": "classic", "frontDoor": map[string]any{"title": "Welcome"}}
	redirects := []any{
		map[string]any{"from": "/old", "to": "/new", "status": 301},
		map[string]any{"from": "/renamed", "to": "/now", "auto": true},
	}
	apply := func(dst map[string]any, b map[string]any) {
		for k, v := range b {
			if v == nil {
				delete(dst, k)
			} else {
				dst[k] = v
			}
		}
	}
	f.on("GET /orgs/org1/spaces/sp-1", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"space": map[string]any{"id": "sp-1", "type": "BLOG"}}
	})
	f.on("PATCH /orgs/org1/spaces/sp-1/seo", 200, func(r *http.Request, b map[string]any) any { apply(seo, b); return seo })
	f.on("GET /orgs/org1/spaces/sp-1/seo", 200, func(*http.Request, map[string]any) any { return seo })
	f.on("PATCH /orgs/org1/spaces/sp-1/theme", 200, func(r *http.Request, b map[string]any) any { apply(theme, b); return map[string]any{"theme": theme} })
	f.on("GET /orgs/org1/spaces/sp-1/theme", 200, func(*http.Request, map[string]any) any { return map[string]any{"theme": theme} })
	f.on("PATCH /orgs/org1/spaces/sp-1/blog/settings", 200, func(r *http.Request, b map[string]any) any { apply(blog, b); return map[string]any{"settings": blog} })
	f.on("GET /orgs/org1/spaces/sp-1/blog/settings", 200, func(*http.Request, map[string]any) any { return map[string]any{"settings": blog} })
	f.on("PATCH /orgs/org1/spaces/sp-1/preset", 200, func(r *http.Request, b map[string]any) any {
		if v, ok := b["preset"]; ok {
			preset["preset"] = v
		}
		return preset
	})
	f.on("GET /orgs/org1/spaces/sp-1/preset", 200, func(*http.Request, map[string]any) any { return preset })
	f.on("PUT /orgs/org1/spaces/sp-1/redirects", 200, func(r *http.Request, b map[string]any) any {
		redirects = append([]any{}, b["redirects"].([]any)...)
		redirects = append(redirects, map[string]any{"from": "/renamed", "to": "/now", "auto": true})
		return map[string]any{"redirects": redirects}
	})
	f.on("GET /orgs/org1/spaces/sp-1/redirects", 200, func(*http.Request, map[string]any) any { return map[string]any{"data": redirects} })

	d := newDriver[spaceSettingsModel](t, f, NewSpaceSettingsResource)
	plan := emptySettings()
	plan.Preset = S("wide")
	plan.SEOJSON = S(`{"seoTitle":"Docs"}`)
	plan.ThemeJSON = S(`{"primaryColor":"#0b835b"}`)
	plan.BlogSettingsJSON = S(`{"whoCanPost":"anyone"}`)
	r1, _ := types.ObjectValue(redirectTypes, map[string]attr.Value{"from": S("/old"), "to": S("/new"), "type": types.StringNull(), "status": types.Int64Value(301), "pass_query": types.BoolNull()})
	plan.Redirects, _ = types.ListValue(types.ObjectType{AttrTypes: redirectTypes}, []attr.Value{r1})

	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if state.ID.ValueString() != "sp-1" {
		t.Fatalf("id = %v", state.ID)
	}
	// Redirects: the platform's own `auto` rule is not part of the managed table.
	if n := len(state.Redirects.Elements()); n != 1 {
		t.Fatalf("redirects = %d", n)
	}
	if rb := f.find("PUT", "/orgs/org1/spaces/sp-1/redirects")[0].Body["redirects"].([]any)[0].(map[string]any); rb["from"] != "/old" || rb["status"] == nil {
		t.Fatalf("redirect body = %v", rb)
	}
	// The blog door asked the space for its type first and it was a BLOG.
	if len(f.find("PATCH", "/orgs/org1/spaces/sp-1/blog/settings")) != 1 {
		t.Fatal("blog settings not written")
	}
	if state.SEOJSON.ValueString() != `{"seoTitle":"Docs"}` {
		t.Fatalf("seo_json = %s", state.SEOJSON.ValueString())
	}

	// Drift in a declared theme key shows; the undeclared fontBody does not.
	theme["primaryColor"] = "#ff0000"
	read, _, diags := d.read(state)
	mustNoErr(t, diags)
	if read.ThemeJSON.ValueString() != `{"primaryColor":"#ff0000"}` {
		t.Fatalf("theme drift = %s", read.ThemeJSON.ValueString())
	}
	theme["primaryColor"] = "#0b835b"

	// Removing a declared SEO key clears it; unchanged parts are not re-sent.
	up := state
	up.SEOJSON = S(`{}`)
	f.reset()
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	pb := f.find("PATCH", "/orgs/org1/spaces/sp-1/seo")[0].Body
	if v, has := pb["seoTitle"]; !has || v != nil {
		t.Fatalf("seo patch = %v", pb)
	}
	if len(f.find("PATCH", "/orgs/org1/spaces/sp-1/theme")) != 0 || len(f.find("PUT", "/orgs/org1/spaces/sp-1/redirects")) != 0 {
		t.Fatal("unchanged parts must not be written")
	}

	// Destroy resets what was declared.
	f.reset()
	mustNoErr(t, d.delete(state))
	if v, has := f.find("PATCH", "/orgs/org1/spaces/sp-1/theme")[0].Body["primaryColor"]; !has || v != nil {
		t.Fatal("destroy must clear declared theme keys")
	}
	if rb := f.find("PUT", "/orgs/org1/spaces/sp-1/redirects")[0].Body["redirects"].([]any); len(rb) != 0 {
		t.Fatalf("destroy must empty the redirect table: %v", rb)
	}
	if len(f.find("PATCH", "/orgs/org1/spaces/sp-1/preset")) != 0 {
		t.Fatal("the layout names have no reset")
	}
}

func TestSpaceSettingsRefusesADoorOfAnotherType(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET /orgs/org1/spaces/sp-1", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"space": map[string]any{"id": "sp-1", "type": "DOCS"}}
	})
	d := newDriver[spaceSettingsModel](t, f, NewSpaceSettingsResource)
	plan := emptySettings()
	plan.BlogSettingsJSON = S(`{"whoCanPost":"anyone"}`)
	_, diags := d.create(plan)
	mustErr(t, diags, "applies to BLOG spaces; this space is DOCS")
	if diags.Errors()[0].Summary() != "Wrong kind of space" {
		t.Fatalf("the error must name the attribute, not the API: %q", diags.Errors()[0].Summary())
	}
}

func TestSpaceSettingsValidation(t *testing.T) {
	d := newDriver[spaceSettingsModel](t, newFakeAPI(t), NewSpaceSettingsResource)
	for name, c := range map[string]struct {
		attr string
		v    types.String
		ok   bool
	}{
		"theme key ok":      {"theme_json", S(`{"primaryColor":"#000000"}`), true},
		"theme key unknown": {"theme_json", S(`{"primaryColour":"#000000"}`), false},
		"seo key unknown":   {"seo_json", S(`{"title":"x"}`), false},
		"seo ok":            {"seo_json", S(`{"seoTitle":"x"}`), true},
		"blog key unknown":  {"blog_settings_json", S(`{"moderate":true}`), false},
		"status ok":         {"status_settings_json", S(`{"autoIncident":true}`), true},
		"forum unknown":     {"forum_settings_json", S(`{"nope":1}`), false},
		"preset too long":   {"preset", S("0123456789012345678901234567890123456789x"), false},
	} {
		if errs := d.attrErrors(c.attr, c.v); (len(errs) == 0) != c.ok {
			t.Errorf("%s: %v", name, errs)
		}
	}
	// Redirect rules.
	nested := func(from, to string, typ string) spaceSettingsModel {
		m := emptySettings()
		r, _ := types.ObjectValue(redirectTypes, map[string]attr.Value{"from": S(from), "to": S(to), "type": S(typ), "status": types.Int64Null(), "pass_query": types.BoolNull()})
		m.Redirects, _ = types.ListValue(types.ObjectType{AttrTypes: redirectTypes}, []attr.Value{r})
		return m
	}
	mustNoErr(t, d.validate(nested("/a", "/b", "redirect")))
	mustNoErr(t, d.validate(nested("/a", "https://x.test/b", "redirect")))
	mustErr(t, d.validate(nested("/a", "https://x.test/b", "rewrite")), "not a proxy")
	for _, v := range []string{"a", "//a", "/a b", "/a?x=1", "/a#frag", ""} {
		if errs := runValidators(d.ctx, redirectNestedFrom(), "from", S(v)); len(errs) == 0 {
			t.Errorf("from %q must be refused", v)
		}
	}
	if errs := runValidators(d.ctx, redirectNestedFrom(), "from", S("/ok/*")); len(errs) != 0 {
		t.Errorf("wildcard from: %v", errs)
	}
}
