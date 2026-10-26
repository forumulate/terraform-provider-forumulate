package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func nullCollection() collectionModel {
	return collectionModel{
		ID: types.StringUnknown(), Slug: types.StringUnknown(), IsDefault: types.BoolUnknown(),
		Nav: types.ObjectNull(attrTypesOf(collectionNavFields)), SEO: types.ObjectNull(attrTypesOf(collectionSEOFields)),
		Translations: types.MapNull(types.ObjectType{AttrTypes: nameDescAttrTypes}),
	}
}

func TestCollectionCRUD(t *testing.T) {
	f := newFakeAPI(t)
	row := map[string]any{
		"id": "col-1", "name": "Guides", "slug": "guides", "description": nil, "coverUrl": nil, "parentId": nil,
		"isDefault": false, "nav": map[string]any{"icon": "book", "collapsed": true, "navScope": "shared"},
		"seo": map[string]any{}, "translations": map[string]any{},
	}
	f.on("POST /orgs/org1/spaces/sp-1/collections", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"collection": row, "data": row}
	})
	f.on("GET /orgs/org1/spaces/sp-1/collections", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{map[string]any{"id": "other"}, row}, "page": map[string]any{"hasMore": false}}
	})
	f.on("PATCH /orgs/org1/spaces/sp-1/collections/col-1", 200, func(r *http.Request, b map[string]any) any {
		if n, ok := b["name"].(string); ok {
			row["name"] = n
		}
		return map[string]any{"collection": row}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1/collections/col-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })

	d := newDriver[collectionModel](t, f, NewCollectionResource)
	plan := nullCollection()
	plan.SpaceID, plan.Name = S("sp-1"), S("Guides")
	navAt := attrTypesOf(collectionNavFields)
	nav := map[string]attr.Value{}
	for k, tp := range navAt {
		switch tp {
		case types.BoolType:
			nav[k] = types.BoolNull()
		case types.StringType:
			nav[k] = types.StringNull()
		default:
			nav[k] = types.SetNull(types.StringType)
		}
	}
	nav["icon"], nav["collapsed"] = S("book"), types.BoolValue(true)
	plan.Nav, _ = types.ObjectValue(navAt, nav)

	state, diags := d.create(plan)
	mustNoErr(t, diags)
	body := f.find("POST", "/orgs/org1/spaces/sp-1/collections")[0].Body
	navBody, _ := body["nav"].(map[string]any)
	if body["name"] != "Guides" || navBody["icon"] != "book" || navBody["collapsed"] != true || len(navBody) != 2 {
		t.Fatalf("create body = %v", body)
	}
	if _, has := body["slug"]; has {
		t.Fatalf("an unset slug must be left to the API: %v", body)
	}
	if state.ID.ValueString() != "col-1" || state.Slug.ValueString() != "guides" {
		t.Fatalf("state = %+v", state)
	}

	// Drift: the nav icon was changed in the studio.
	row["nav"].(map[string]any)["icon"] = "code"
	read, gone, diags := d.read(state)
	mustNoErr(t, diags)
	if gone || read.Nav.Attributes()["icon"].(types.String).ValueString() != "code" {
		t.Fatalf("nav drift not detected: %v", read.Nav)
	}
	// The API's other nav keys (navScope) are not drift: they were not declared.
	if !read.Nav.Attributes()["nav_scope"].IsNull() {
		t.Fatalf("undeclared nav key leaked into state: %v", read.Nav)
	}
	row["nav"].(map[string]any)["icon"] = "book"

	// Update drops the nav icon: sent as null so the API clears it.
	up := state
	up.Name = S("Guides 2")
	nav2 := map[string]attr.Value{}
	for k, v := range nav {
		nav2[k] = v
	}
	nav2["icon"] = types.StringNull()
	up.Nav, _ = types.ObjectValue(navAt, nav2)
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	pb := f.find("PATCH", "/orgs/org1/spaces/sp-1/collections/col-1")[0].Body
	nb, _ := pb["nav"].(map[string]any)
	if v, has := nb["icon"]; !has || v != nil {
		t.Fatalf("removed nav.icon must be null in the PATCH: %v", pb)
	}

	mustNoErr(t, d.delete(state))

	// Gone from the list = removed from state.
	row2 := state
	row2.ID = S("missing")
	_, gone, diags = d.read(row2)
	mustNoErr(t, diags)
	if !gone {
		t.Fatal("a collection missing from the list must leave state")
	}

	imp, _, diags := d.importState("sp-1/col-1")
	mustNoErr(t, diags)
	if imp.SpaceID.ValueString() != "sp-1" || imp.Name.ValueString() != "Guides 2" {
		t.Fatalf("import = %+v", imp)
	}
	_, _, diags = d.importState("nonsense")
	mustErr(t, diags, "<space_id>/<collection_id>")
}

func TestCollectionValidation(t *testing.T) {
	d := newDriver[collectionModel](t, newFakeAPI(t), NewCollectionResource)
	if errs := d.attrErrors("name", S("")); len(errs) == 0 {
		t.Error("empty name")
	}
	if errs := d.attrErrors("slug", S("Not A Slug")); len(errs) == 0 {
		t.Error("bad slug")
	}
	if errs := d.attrErrors("parent_id", S("not-a-uuid")); len(errs) == 0 {
		t.Error("parent must be a uuid")
	}
	if errs := d.attrErrors("description", S(string(make([]byte, 501)))); len(errs) == 0 {
		t.Error("description over 500")
	}
	if errs := iconName(collectionIcons); errs == nil {
		t.Fatal("no validator")
	}
}
