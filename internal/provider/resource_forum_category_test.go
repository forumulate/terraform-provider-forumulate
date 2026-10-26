package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// planCategory builds a plan the way Terraform does: unset Optional+Computed
// attributes unknown, unset Optional ones null.
func planCategory(name string) categoryModel {
	m := categoryModel{
		ID: types.StringUnknown(), SpaceID: S("sp-1"), Name: S(name),
		Slug: types.StringUnknown(), Description: types.StringUnknown(), SortOrder: types.Int64Unknown(),
		DefaultNotificationLevel: types.StringUnknown(), MinimumRequiredTags: types.Int64Unknown(),
		DefaultTags: types.SetUnknown(types.StringType), AllowedTags: types.SetUnknown(types.StringType),
		ShowSubcategoryList: types.BoolUnknown(), RequiredTagGroups: types.ListUnknown(types.ObjectType{AttrTypes: tagGroupAttrTypes}),
		RequireTopicApproval: types.BoolUnknown(), RequireReplyApproval: types.BoolUnknown(), SearchPriority: types.StringUnknown(),
		ExpertUserIDs: types.SetUnknown(types.StringType), TopicKind: types.StringUnknown(), TopicCount: types.Int64Unknown(),
		Translations: types.MapNull(types.ObjectType{AttrTypes: nameDescAttrTypes}),
	}
	// Every other attribute is Optional-only: null.
	return m
}

func categoryRow(name, color string) map[string]any {
	return map[string]any{
		"id": "cat-1", "name": name, "slug": "general", "description": "", "color": color, "colorDark": nil, "textColor": nil, "textColorDark": nil,
		"parentId": nil, "sortOrder": 0, "defaultNotificationLevel": "normal", "minimumRequiredTags": 0, "defaultTags": []any{}, "allowedTags": []any{},
		"defaultSort": nil, "logoUrl": nil, "backgroundUrl": nil, "numFeaturedTopics": nil, "showSubcategoryList": false, "autoCloseHours": nil,
		"defaultSlowModeSeconds": nil, "requiredTagGroups": []any{}, "navigateToFirstPost": nil, "defaultListFilter": nil,
		"requireTopicApproval": false, "requireReplyApproval": false, "votingEnabled": nil, "searchPriority": "normal", "postGuidelines": nil,
		"expertUserIds": []any{}, "locale": nil, "icon": nil, "emoji": nil, "style": nil, "subcategoryListStyle": nil, "defaultTopPeriod": nil,
		"sortAscending": nil, "aboutTopicId": nil, "topicKind": "discussion", "topicTemplate": nil, "translations": map[string]any{},
		"topicCount": 3,
	}
}

func TestForumCategoryCRUD(t *testing.T) {
	f := newFakeAPI(t)
	row := categoryRow("General", "#0b835b")
	f.on("POST /orgs/org1/spaces/sp-1/forum/categories", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"category": row, "data": row, "warnings": []any{map[string]any{"field": "textColor", "message": "Low contrast", "ratio": 2.1, "required": 4.5}}}
	})
	f.on("GET /orgs/org1/spaces/sp-1/forum/categories", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{row}}
	})
	f.on("PATCH /orgs/org1/spaces/sp-1/forum/categories/cat-1", 200, func(r *http.Request, b map[string]any) any {
		if n, ok := b["name"].(string); ok {
			row["name"] = n
		}
		return map[string]any{"category": row}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1/forum/categories/cat-1", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"ok": true, "topics": 3, "movedTo": "cat-2"}
	})
	d := newDriver[categoryModel](t, f, NewForumCategoryResource)

	plan := planCategory("General")
	// A colour without the '#': the API stores it normalised, and the
	// configured spelling must survive the round trip.
	plan.Color = S("0b835b")
	plan.TextColor = S("ffffff")
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if len(diags.Warnings()) != 1 {
		t.Fatalf("contrast warning not surfaced: %v", diags)
	}
	body := f.find("POST", "/orgs/org1/spaces/sp-1/forum/categories")[0].Body
	if body["name"] != "General" || body["color"] != "0b835b" {
		t.Fatalf("body = %v", body)
	}
	if _, has := body["sortOrder"]; has {
		t.Fatalf("unknown computed attributes must not be sent: %v", body)
	}
	if state.Color.ValueString() != "0b835b" || state.Slug.ValueString() != "general" || state.TopicCount.ValueInt64() != 3 {
		t.Fatalf("state = color %q slug %q", state.Color.ValueString(), state.Slug.ValueString())
	}

	// Drift in a real colour change shows.
	row["color"] = "#ff0000"
	read, _, diags := d.read(state)
	mustNoErr(t, diags)
	if read.Color.ValueString() != "#ff0000" {
		t.Fatalf("colour drift not detected: %q", read.Color.ValueString())
	}
	row["color"] = "#0b835b"

	// Removing the colour sends null.
	up := state
	up.Color = types.StringNull()
	up.Name = S("Renamed")
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	pb := f.find("PATCH", "/orgs/org1/spaces/sp-1/forum/categories/cat-1")[0].Body
	if v, has := pb["color"]; !has || v != nil {
		t.Fatalf("a removed colour must be null: %v", pb)
	}

	// Delete passes moveTo / force.
	state.DeleteMoveTopicsTo = S("cat-2")
	mustNoErr(t, d.delete(state))
	if q := f.find("DELETE", "/orgs/org1/spaces/sp-1/forum/categories/cat-1"); q[0].Query != "moveTo=cat-2" {
		t.Fatalf("delete query = %q", q[0].Query)
	}

	_, gone, diags := d.read(categoryModel{ID: S("nope"), SpaceID: S("sp-1"),
		DefaultTags: types.SetNull(types.StringType), AllowedTags: types.SetNull(types.StringType), ExpertUserIDs: types.SetNull(types.StringType),
		RequiredTagGroups: types.ListNull(types.ObjectType{AttrTypes: tagGroupAttrTypes}), Translations: types.MapNull(types.ObjectType{AttrTypes: nameDescAttrTypes})})
	mustNoErr(t, diags)
	if !gone {
		t.Fatal("missing category must leave state")
	}
}

func TestForumCategoryTranslationsAndTagGroups(t *testing.T) {
	f := newFakeAPI(t)
	row := categoryRow("General", "#0b835b")
	row["translations"] = map[string]any{"es": map[string]any{"name": "General (es)", "description": "Hola"}}
	row["requiredTagGroups"] = []any{map[string]any{"tagGroupId": "tg-1", "minCount": 2}}
	f.on("POST /orgs/org1/spaces/sp-1/forum/categories", 201, func(r *http.Request, b map[string]any) any {
		return map[string]any{"category": row}
	})
	d := newDriver[categoryModel](t, f, NewForumCategoryResource)
	plan := planCategory("General")
	es, _ := types.ObjectValue(nameDescAttrTypes, map[string]attr.Value{"name": S("General (es)"), "description": S("Hola")})
	plan.Translations, _ = types.MapValue(types.ObjectType{AttrTypes: nameDescAttrTypes}, map[string]attr.Value{"es": es})
	tg, _ := types.ObjectValue(tagGroupAttrTypes, map[string]attr.Value{"tag_group_id": S("tg-1"), "min_count": types.Int64Value(2)})
	plan.RequiredTagGroups, _ = types.ListValue(types.ObjectType{AttrTypes: tagGroupAttrTypes}, []attr.Value{tg})
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	body := f.last().Body
	tr, _ := body["translations"].(map[string]any)
	if tr["es"].(map[string]any)["name"] != "General (es)" {
		t.Fatalf("translations body = %v", body)
	}
	groups := body["requiredTagGroups"].([]any)
	if groups[0].(map[string]any)["tagGroupId"] != "tg-1" {
		t.Fatalf("tag groups body = %v", body)
	}
	if len(state.Translations.Elements()) != 1 || len(state.RequiredTagGroups.Elements()) != 1 {
		t.Fatalf("state = %+v", state)
	}
}

func TestForumCategoryValidation(t *testing.T) {
	d := newDriver[categoryModel](t, newFakeAPI(t), NewForumCategoryResource)
	cases := map[string]struct {
		attr string
		v    interface{}
		ok   bool
	}{
		"notification bad": {"default_notification_level", S("loud"), false},
		"notification ok":  {"default_notification_level", S("watching_first_post"), true},
		"sort bad":         {"default_sort", S("new"), false},
		"icon custom":      {"icon", S("/img/x.svg"), true},
		"icon named ok":    {"icon", S("book"), true},
		"icon named bad":   {"icon", S("not-an-icon"), false},
		"emoji ascii":      {"emoji", S("x"), false},
		"emoji ok":         {"emoji", S("🚀"), true},
		"locale bad":       {"locale", S("EN_us"), false},
		"guidelines long":  {"post_guidelines", S(string(make([]byte, 2001))), false},
		"top period bad":   {"default_top_period", S("decade"), false},
		"search bad":       {"search_priority", S("max"), false},
		"sort order big":   {"sort_order", types.Int64Value(10000), false},
		"min tags":         {"minimum_required_tags", types.Int64Value(13), false},
		"slow mode ok":     {"default_slow_mode_seconds", types.Int64Value(86400), true},
		"auto close big":   {"auto_close_hours", types.Int64Value(9000), false},
		"featured big":     {"num_featured_topics", types.Int64Value(51), false},
	}
	for name, c := range cases {
		errs := d.attrErrors(c.attr, c.v.(attr.Value))
		if (len(errs) == 0) != c.ok {
			t.Errorf("%s: %v", name, errs)
		}
	}
	tags := make([]attr.Value, 13)
	for i := range tags {
		tags[i] = S("t")
	}
	if errs := d.attrErrors("default_tags", types.SetValueMust(types.StringType, []attr.Value{S("a"), S("b")})); len(errs) != 0 {
		t.Errorf("two tags: %v", errs)
	}
	cfg := planCategory("x")
	cfg.DeleteForce = types.BoolValue(true)
	cfg.DeleteMoveTopicsTo = S("cat-2")
	mustErr(t, d.validate(cfg), "not both")
}
