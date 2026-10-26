package provider

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func nullPageMeta() map[string]attr.Value {
	m := map[string]attr.Value{}
	for _, f := range pageMetaFields {
		m[f.tf] = nullOf(f)
	}
	return m
}

func TestPageCRUD(t *testing.T) {
	f := newFakeAPI(t)
	// The API rewrites "* a" as "- a": the normalised Markdown differs from
	// what was sent, and that must not read as drift.
	remote := "- a\n- b\n"
	status := "draft"
	title := "Hello"
	slug := "hello"
	meta := map[string]any{}
	pageJSON := func() map[string]any {
		return map[string]any{
			"id": "pg-1", "slug": slug, "title": title, "status": status, "locale": "default", "versionKey": "current",
			"contentHash": "h1", "url": "https://x.test/docs/hello", "updatedAt": "2027-01-01T00:00:00Z",
			"parentId": nil, "collection": map[string]any{"id": "col-1", "name": "Guides"},
		}
	}
	f.on("POST /orgs/org1/spaces/sp-1/collections/col-1/pages", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"page": map[string]any{"id": "pg-1", "slug": slug, "title": title}}
	})
	f.on("GET /orgs/org1/spaces/sp-1/pages/pg-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"page": pageJSON()} })
	f.on("GET /orgs/org1/spaces/sp-1/pages/pg-1/metadata", 200, func(*http.Request, map[string]any) any { return map[string]any{"metadata": meta} })
	f.on("PATCH /orgs/org1/spaces/sp-1/pages/pg-1/metadata", 200, func(r *http.Request, b map[string]any) any {
		for k, v := range b {
			if v == nil {
				delete(meta, k)
			} else {
				meta[k] = v
			}
		}
		return map[string]any{"metadata": meta}
	})
	f.on("POST /orgs/org1/spaces/sp-1/pages/pg-1/publish", 200, func(*http.Request, map[string]any) any { status = "published"; return map[string]any{} })
	f.on("POST /orgs/org1/spaces/sp-1/pages/pg-1/unpublish", 200, func(*http.Request, map[string]any) any { status = "draft"; return map[string]any{} })
	f.on("PUT /orgs/org1/spaces/sp-1/pages/pg-1/content", 200, func(r *http.Request, b map[string]any) any {
		remote = b["body"].(string)
		return map[string]any{}
	})
	f.on("PATCH /orgs/org1/spaces/sp-1/pages/pg-1", 200, func(r *http.Request, b map[string]any) any {
		title, slug = b["title"].(string), b["slug"].(string)
		return map[string]any{"ok": true, "slug": slug}
	})
	f.on("POST /orgs/org1/spaces/sp-1/pages/pg-1/reorder", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })
	f.on("DELETE /orgs/org1/spaces/sp-1/pages/pg-1", 200, func(*http.Request, map[string]any) any { return map[string]any{} })
	f.on("DELETE /orgs/org1/spaces/sp-1/pages/pg-1/destroy", 200, func(*http.Request, map[string]any) any { return map[string]any{} })
	f.onText("GET /orgs/org1/spaces/sp-1/pages/pg-1/draft/markdown", func() string { return remote })

	d := newDriver[pageModel](t, f, NewPageResource)
	mk := nullPageMeta()
	mk["seo_title"] = S("Hello page")
	mk["tags"] = setOfStrings([]string{"intro"})
	metaObj, _ := types.ObjectValue(attrTypesOf(pageMetaFields), mk)
	plan := pageModel{
		ID: types.StringUnknown(), SpaceID: S("sp-1"), CollectionID: S("col-1"), Slug: S("hello"), Title: S("Hello"),
		Body: S("* a\n* b"), Locale: types.StringUnknown(), VersionKey: types.StringUnknown(), Published: types.BoolValue(true),
		Metadata: metaObj, BodyHash: types.StringUnknown(), ContentHash: types.StringUnknown(), URL: types.StringUnknown(),
		UpdatedAt: types.StringUnknown(),
	}
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	post := f.find("POST", "/orgs/org1/spaces/sp-1/collections/col-1/pages")[0].Body
	if post["body"] != "* a\n* b" || post["slug"] != "hello" || post["onSlugTaken"] != "fail" {
		t.Fatalf("create body = %v", post)
	}
	if len(f.find("POST", "/orgs/org1/spaces/sp-1/pages/pg-1/publish")) != 1 {
		t.Fatal("published pages are published after creation")
	}
	pm := f.find("PATCH", "/orgs/org1/spaces/sp-1/pages/pg-1/metadata")[0].Body
	if pm["seoTitle"] != "Hello page" || pm["tags"] == nil {
		t.Fatalf("metadata body = %v", pm)
	}
	if state.Body.ValueString() != "* a\n* b" || !state.Published.ValueBool() || state.URL.ValueString() == "" {
		t.Fatalf("state = %+v", state)
	}

	// The API's normalisation is not drift ...
	read, gone, diags := d.read(state)
	mustNoErr(t, diags)
	if gone || read.Body.ValueString() != "* a\n* b" {
		t.Fatalf("normalised body read as drift: %q", read.Body.ValueString())
	}
	// ... an edit in the studio is.
	remote = "- a\n- b\n- c\n"
	read, _, diags = d.read(state)
	mustNoErr(t, diags)
	if strings.TrimSpace(read.Body.ValueString()) != "- a\n- b\n- c" {
		t.Fatalf("studio edit not detected: %q", read.Body.ValueString())
	}
	remote = "- a\n- b\n"
	// Unpublished outside Terraform.
	status = "draft"
	read, _, _ = d.read(state)
	if read.Published.ValueBool() {
		t.Fatal("unpublish outside Terraform must read as published=false")
	}
	status = "published"

	// Update: new body re-publishes; clearing seo_title nulls it.
	up := state
	up.Body = S("New body")
	mk2 := nullPageMeta()
	mk2["tags"] = setOfStrings([]string{"intro"})
	up.Metadata, _ = types.ObjectValue(attrTypesOf(pageMetaFields), mk2)
	f.reset()
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	if c := f.find("PUT", "/orgs/org1/spaces/sp-1/pages/pg-1/content"); len(c) != 1 || c[0].Body["body"] != "New body" {
		t.Fatalf("content calls = %+v", c)
	}
	if v, has := f.find("PATCH", "/orgs/org1/spaces/sp-1/pages/pg-1/metadata")[0].Body["seoTitle"]; !has || v != nil {
		t.Fatal("a removed metadata attribute must be sent as null")
	}
	if len(f.find("POST", "/orgs/org1/spaces/sp-1/pages/pg-1/publish")) != 1 {
		t.Fatal("a changed body of a published page is published again")
	}

	// Move: collection and parent go through reorder.
	mv := state
	mv.ParentID = S("pg-0")
	f.reset()
	_, diags = d.update(mv, state)
	mustNoErr(t, diags)
	rb := f.find("POST", "/orgs/org1/spaces/sp-1/pages/pg-1/reorder")[0].Body
	if rb["parentId"] != "pg-0" || rb["collectionId"] != "col-1" {
		t.Fatalf("reorder body = %v", rb)
	}

	// Unpublish by configuration.
	un := state
	un.Published = types.BoolValue(false)
	f.reset()
	_, diags = d.update(un, state)
	mustNoErr(t, diags)
	if len(f.find("POST", "/orgs/org1/spaces/sp-1/pages/pg-1/unpublish")) != 1 {
		t.Fatal("published=false must unpublish")
	}

	// Delete, then destroy with confirm=<slug>.
	f.reset()
	mustNoErr(t, d.delete(state))
	state.DestroyOnDelete = types.BoolValue(true)
	mustNoErr(t, d.delete(state))
	if q := f.find("DELETE", "/orgs/org1/spaces/sp-1/pages/pg-1/destroy"); len(q) != 1 || q[0].Query != "confirm=hello" {
		t.Fatalf("destroy = %+v", q)
	}
}

func TestPageSlugTakenIsAnErrorOnTheSlugAttribute(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST /orgs/org1/spaces/sp-1/collections/col-1/pages", 409, func(*http.Request, map[string]any) any {
		e := fail(409, "page.slug_taken", "That slug is taken.", "slug")
		return e
	})
	d := newDriver[pageModel](t, f, NewPageResource)
	plan := pageModel{
		ID: types.StringUnknown(), SpaceID: S("sp-1"), CollectionID: S("col-1"), Slug: S("hello"), Title: S("Hello"),
		Locale: types.StringUnknown(), VersionKey: types.StringUnknown(), Published: types.BoolValue(true),
		Metadata: types.ObjectNull(attrTypesOf(pageMetaFields)), BodyHash: types.StringUnknown(), ContentHash: types.StringUnknown(),
		URL: types.StringUnknown(), UpdatedAt: types.StringUnknown(),
	}
	_, diags := d.create(plan)
	mustErr(t, diags, "page.slug_taken")
	if diags.Errors()[0].Summary() != "Forumulate API: creating page" {
		t.Fatalf("summary = %q", diags.Errors()[0].Summary())
	}
}

func TestPageValidation(t *testing.T) {
	d := newDriver[pageModel](t, newFakeAPI(t), NewPageResource)
	for name, c := range map[string]struct {
		attr string
		v    types.String
		ok   bool
	}{
		"title empty":       {"title", S(""), false},
		"title 300":         {"title", S(strings.Repeat("a", 300)), true},
		"title 301":         {"title", S(strings.Repeat("a", 301)), false},
		"slug bad":          {"slug", S("Hello World"), false},
		"slug 80":           {"slug", S(strings.Repeat("a", 80)), true},
		"slug 81":           {"slug", S(strings.Repeat("a", 81)), false},
		"collection not id": {"collection_id", S("guides"), false},
		"locale default":    {"locale", S("default"), true},
		"locale es":         {"locale", S("pt-br"), true},
		"locale upper":      {"locale", S("EN"), false},
		"version key bad":   {"version_key", S("V1"), false},
		"version key ok":    {"version_key", S("v1.2"), true},
		"preset bad":        {"preset", S("Not A Kind"), false},
		"metadata_json arr": {"metadata_json", S("[]"), false},
	} {
		if errs := d.attrErrors(c.attr, c.v); (len(errs) == 0) != c.ok {
			t.Errorf("%s: %v", name, errs)
		}
	}
}
