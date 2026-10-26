package provider

import (
	"context"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type categoryModel struct {
	ID                       types.String `tfsdk:"id"`
	SpaceID                  types.String `tfsdk:"space_id"`
	Name                     types.String `tfsdk:"name"`
	Slug                     types.String `tfsdk:"slug"`
	Description              types.String `tfsdk:"description"`
	Color                    types.String `tfsdk:"color"`
	ColorDark                types.String `tfsdk:"color_dark"`
	TextColor                types.String `tfsdk:"text_color"`
	TextColorDark            types.String `tfsdk:"text_color_dark"`
	ParentID                 types.String `tfsdk:"parent_id"`
	SortOrder                types.Int64  `tfsdk:"sort_order"`
	DefaultNotificationLevel types.String `tfsdk:"default_notification_level"`
	TopicTemplateJSON        types.String `tfsdk:"topic_template_json"`
	MinimumRequiredTags      types.Int64  `tfsdk:"minimum_required_tags"`
	DefaultTags              types.Set    `tfsdk:"default_tags"`
	AllowedTags              types.Set    `tfsdk:"allowed_tags"`
	DefaultSort              types.String `tfsdk:"default_sort"`
	LogoURL                  types.String `tfsdk:"logo_url"`
	BackgroundURL            types.String `tfsdk:"background_url"`
	NumFeaturedTopics        types.Int64  `tfsdk:"num_featured_topics"`
	ShowSubcategoryList      types.Bool   `tfsdk:"show_subcategory_list"`
	AutoCloseHours           types.Int64  `tfsdk:"auto_close_hours"`
	DefaultSlowModeSeconds   types.Int64  `tfsdk:"default_slow_mode_seconds"`
	RequiredTagGroups        types.List   `tfsdk:"required_tag_groups"`
	NavigateToFirstPost      types.Bool   `tfsdk:"navigate_to_first_post"`
	DefaultListFilter        types.String `tfsdk:"default_list_filter"`
	RequireTopicApproval     types.Bool   `tfsdk:"require_topic_approval"`
	RequireReplyApproval     types.Bool   `tfsdk:"require_reply_approval"`
	VotingEnabled            types.Bool   `tfsdk:"voting_enabled"`
	SearchPriority           types.String `tfsdk:"search_priority"`
	PostGuidelines           types.String `tfsdk:"post_guidelines"`
	ExpertUserIDs            types.Set    `tfsdk:"expert_user_ids"`
	Locale                   types.String `tfsdk:"locale"`
	Icon                     types.String `tfsdk:"icon"`
	Emoji                    types.String `tfsdk:"emoji"`
	Style                    types.String `tfsdk:"style"`
	SubcategoryListStyle     types.String `tfsdk:"subcategory_list_style"`
	DefaultTopPeriod         types.String `tfsdk:"default_top_period"`
	SortAscending            types.Bool   `tfsdk:"sort_ascending"`
	AboutTopicID             types.String `tfsdk:"about_topic_id"`
	TopicKind                types.String `tfsdk:"topic_kind"`
	Translations             types.Map    `tfsdk:"translations"`
	DeleteMoveTopicsTo       types.String `tfsdk:"delete_move_topics_to"`
	DeleteForce              types.Bool   `tfsdk:"delete_force"`
	TopicCount               types.Int64  `tfsdk:"topic_count"`
}

// categoryFields maps the flat attributes to the API's keys.
var categoryFields = []fieldMap{
	{"name", "name", kStr}, {"slug", "slug", kStr}, {"description", "description", kStr},
	{"color", "color", kStr}, {"color_dark", "colorDark", kStr},
	{"text_color", "textColor", kStr}, {"text_color_dark", "textColorDark", kStr},
	{"parent_id", "parentId", kStr}, {"sort_order", "sortOrder", kInt},
	{"default_notification_level", "defaultNotificationLevel", kStr},
	{"minimum_required_tags", "minimumRequiredTags", kInt},
	{"default_tags", "defaultTags", kStrSet}, {"allowed_tags", "allowedTags", kStrSet},
	{"default_sort", "defaultSort", kStr}, {"logo_url", "logoUrl", kStr}, {"background_url", "backgroundUrl", kStr},
	{"num_featured_topics", "numFeaturedTopics", kInt}, {"show_subcategory_list", "showSubcategoryList", kBool},
	{"auto_close_hours", "autoCloseHours", kInt}, {"default_slow_mode_seconds", "defaultSlowModeSeconds", kInt},
	{"navigate_to_first_post", "navigateToFirstPost", kBool}, {"default_list_filter", "defaultListFilter", kStr},
	{"require_topic_approval", "requireTopicApproval", kBool}, {"require_reply_approval", "requireReplyApproval", kBool},
	{"voting_enabled", "votingEnabled", kBool}, {"search_priority", "searchPriority", kStr},
	{"post_guidelines", "postGuidelines", kStr}, {"expert_user_ids", "expertUserIds", kStrSet},
	{"locale", "locale", kStr}, {"icon", "icon", kStr}, {"emoji", "emoji", kStr}, {"style", "style", kStr},
	{"subcategory_list_style", "subcategoryListStyle", kStr}, {"default_top_period", "defaultTopPeriod", kStr},
	{"sort_ascending", "sortAscending", kBool}, {"about_topic_id", "aboutTopicId", kStr}, {"topic_kind", "topicKind", kStr},
}

// categoryClearable are the fields the API clears with null.
var categoryClearable = map[string]bool{
	"color": true, "color_dark": true, "text_color": true, "text_color_dark": true, "parent_id": true,
	"default_sort": true, "logo_url": true, "background_url": true, "num_featured_topics": true,
	"auto_close_hours": true, "default_slow_mode_seconds": true, "navigate_to_first_post": true,
	"default_list_filter": true, "voting_enabled": true, "post_guidelines": true, "locale": true, "icon": true,
	"emoji": true, "style": true, "subcategory_list_style": true, "default_top_period": true,
	"sort_ascending": true, "about_topic_id": true,
}

var tagGroupAttrTypes = map[string]attr.Type{"tag_group_id": types.StringType, "min_count": types.Int64Type}

// emojiOnly mirrors the API's "starts with a pictographic character" rule as
// closely as Go's regexp allows: the first character is not plain ASCII.
func emojiOnly() validator.String {
	return stringCheck{
		desc: "an emoji",
		fn: func(s string) string {
			if s == "" {
				return ""
			}
			r, _ := utf8.DecodeRuneInString(s)
			if r < 0x2000 || utf8.RuneCountInString(s) > 16 {
				return "must be an emoji of at most 16 characters (it is user content, not interface chrome)."
			}
			return ""
		},
	}
}

func optComputedSet(desc string, vs ...validator.Set) schema.SetAttribute {
	return schema.SetAttribute{
		Optional: true, Computed: true, ElementType: types.StringType, Validators: vs,
		PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
		MarkdownDescription: desc,
	}
}

// NewForumCategoryResource is `forumulate_forum_category`.
func NewForumCategoryResource() resource.Resource {
	return newResource(&resourceSpec[categoryModel]{
		suffix: "forum_category",
		what:   "forum category",
		description: "A category of a forum space: its look (colours, icon, logo), who may post and how (approval, slow mode, tags, templates), its defaults for readers, and its names in other languages. " +
			"Destroying a category is refused by the API while it holds topics; set `delete_move_topics_to` (they move there) or `delete_force` (they are left uncategorised). The topics themselves are never deleted.",
		importHelp: "<space_id>/<category_id>",
		paramAlias: map[string]string{"topicTemplate": "topic_template_json", "requiredTagGroups": "required_tag_groups"},
		attrs: map[string]schema.Attribute{
			"id":              computedID("The category's id (a UUID)."),
			"space_id":        reqStr("The forum space (its id or slug). Changing it replaces the category.", replStr, idOrSlug()),
			"name":            reqStr("The category's name, 1 to 100 characters.", nil, length(1, 100)),
			"slug":            optComputedStr("The category's address segment (up to 100 characters). Derived from the name when left out.", length(1, 100), pattern(slugRE, slugReason)),
			"description":     optComputedStr("A description, up to 500 characters.", length(0, 500)),
			"color":           optStr("The category colour: hex with or without `#` (3, 4, 6 or 8 digits) or a CSS colour name, up to 40 characters. The API stores it normalised.", length(1, 40)),
			"color_dark":      optStr("The colour in the dark scheme.", length(1, 40)),
			"text_color":      optStr("The text colour drawn on `color`. The API warns when the pair is under WCAG AA contrast.", length(1, 40)),
			"text_color_dark": optStr("The text colour in the dark scheme.", length(1, 40)),
			"parent_id":       optStr("The parent category's id; leave out for a top-level category.", uuid()),
			"sort_order":      optComputedInt("Position among siblings, 0 to 9999.", between(0, 9999)),
			"default_notification_level": optComputedStr("What readers are set to by default: `watching`, `watching_first_post`, `tracking`, `normal` or `muted`.",
				oneOf("watching", "watching_first_post", "tracking", "normal", "muted")),
			"topic_template_json":   jsonAttr("A template new topics start from (a JSON object)."),
			"minimum_required_tags": optComputedInt("How many tags a new topic must carry, 0 to 12.", between(0, 12)),
			"default_tags":          optComputedSet("Tags applied to new topics (up to 12, 60 characters each).", setOf(0, 12, length(1, 60))),
			"allowed_tags":          optComputedSet("Only these tags may be used here (up to 50); empty allows any.", setOf(0, 50, length(1, 60))),
			"default_sort":          optStr("How the category is sorted by default: `latest`, `top` or `hot`.", oneOf("latest", "top", "hot")),
			"logo_url":              optStr("A logo: an uploaded path or an http(s) URL, up to 2048 characters.", length(1, 2048)),
			"background_url":        optStr("A background image, up to 2048 characters.", length(1, 2048)),
			"num_featured_topics":   schema.Int64Attribute{Optional: true, Validators: []validator.Int64{between(0, 50)}, MarkdownDescription: "How many topics the category page features, 0 to 50."},
			"show_subcategory_list": optComputedBool("Whether the category page lists its subcategories."),
			"auto_close_hours":      schema.Int64Attribute{Optional: true, Validators: []validator.Int64{between(0, 8760)}, MarkdownDescription: "Close topics this many hours after the last reply (0 to 8760); leave out for never."},
			"default_slow_mode_seconds": schema.Int64Attribute{Optional: true, Validators: []validator.Int64{between(0, 86400)},
				MarkdownDescription: "Seconds a member must wait between posts in new topics (0 to 86400)."},
			"required_tag_groups": schema.ListNestedAttribute{
				Optional: true, Computed: true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Tag groups a topic must draw tags from (up to 20).",
				Validators:          []validator.List{listOf(0, 20)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"tag_group_id": schema.StringAttribute{Required: true, Validators: []validator.String{uuid()}, MarkdownDescription: "The tag group's id."},
					"min_count":    schema.Int64Attribute{Required: true, Validators: []validator.Int64{between(1, 12)}, MarkdownDescription: "How many tags from the group, 1 to 12."},
				}},
			},
			"navigate_to_first_post": schema.BoolAttribute{Optional: true, MarkdownDescription: "Open topics at their first post instead of the last unread."},
			"default_list_filter":    optStr("`all` or `none`.", oneOf("all", "none")),
			"require_topic_approval": optComputedBool("New topics wait for a moderator."),
			"require_reply_approval": optComputedBool("New replies wait for a moderator."),
			"voting_enabled":         schema.BoolAttribute{Optional: true, MarkdownDescription: "Let readers vote on topics here."},
			"search_priority":        optComputedStr("`very_low`, `low`, `normal`, `high` or `very_high`.", oneOf("very_low", "low", "normal", "high", "very_high")),
			"post_guidelines":        optStr("Shown to people composing a topic, up to 2000 characters.", length(1, 2000)),
			"expert_user_ids":        optComputedSet("Users whose answers count as expert (up to 20 user ids).", setOf(0, 20, pattern(uuidRE, "must be a UUID."))),
			"locale":                 optStr("The category's language, a code like en or pt-br.", pattern(regexpLocale, "must be a language code like en or pt-br.")),
			"icon":                   optStr("A named icon, or a custom icon (a path or URL with a scheme).", iconName(categoryIcons)),
			"emoji":                  optStr("An emoji for the category (user content). Up to 16 characters.", emojiOnly()),
			"style":                  optStr("How the category is marked: `square`, `icon`, `emoji` or `none`.", oneOf("square", "icon", "emoji", "none")),
			"subcategory_list_style": optStr("`rows`, `boxes` or `chips`.", oneOf("rows", "boxes", "chips")),
			"default_top_period":     optStr("`day`, `week`, `month`, `quarter` or `year`.", oneOf("day", "week", "month", "quarter", "year")),
			"sort_ascending":         schema.BoolAttribute{Optional: true, MarkdownDescription: "Sort the default order ascending."},
			"about_topic_id":         optStr("The topic that describes the category (its id).", uuid()),
			"topic_kind":             optComputedStr("`question` files new topics as questions (Q&A look); `discussion` as discussions.", oneOf("question", "discussion")),
			"translations":           nameDescTranslations("The category's name and description in other languages."),
			"delete_move_topics_to": schema.StringAttribute{
				Optional: true, Validators: []validator.String{uuid()},
				MarkdownDescription: "On destroy, move the category's topics to this category (its id) instead of refusing.",
			},
			"delete_force": schema.BoolAttribute{Optional: true, MarkdownDescription: "On destroy, leave the category's topics uncategorised instead of refusing."},
			"topic_count":  schema.Int64Attribute{Computed: true, MarkdownDescription: "How many topics the category holds."},
		},
		create: func(ctx context.Context, c *client.Client, m *categoryModel, d *diag.Diagnostics) error {
			body := bodyFromFields(m, nil, categoryFields, categoryClearable)
			if known(m.TopicTemplateJSON) {
				setJSON(body, "topicTemplate", m.TopicTemplateJSON, false)
			}
			if known(m.RequiredTagGroups) {
				body["requiredTagGroups"] = tagGroupsBody(m.RequiredTagGroups)
			}
			if known(m.Translations) {
				body["translations"] = translationsBody(m.Translations)
			}
			out, err := c.DoObject(ctx, http.MethodPost, c.SpacePath(m.SpaceID.ValueString(), "forum", "categories"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			applyCategory(m, client.Unwrap(out, "category"))
			warnContrast(d, out)
			return nil
		},
		created: func(m *categoryModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *categoryModel, d *diag.Diagnostics) error {
			row, err := findRow(ctx, c, c.SpacePath(m.SpaceID.ValueString(), "forum", "categories"), "categories", "id", m.ID.ValueString(), "forum category")
			if err != nil {
				return err
			}
			applyCategory(m, row)
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *categoryModel, d *diag.Diagnostics) error {
			body := bodyFromFields(plan, prior, categoryFields, categoryClearable)
			if !plan.TopicTemplateJSON.Equal(prior.TopicTemplateJSON) {
				setJSON(body, "topicTemplate", plan.TopicTemplateJSON, true)
			}
			if known(plan.RequiredTagGroups) && !plan.RequiredTagGroups.Equal(prior.RequiredTagGroups) {
				body["requiredTagGroups"] = tagGroupsBody(plan.RequiredTagGroups)
			}
			if known(plan.Translations) || known(prior.Translations) {
				body["translations"] = translationsBody(plan.Translations)
			}
			plan.ID = prior.ID
			out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(plan.SpaceID.ValueString(), "forum", "categories", prior.ID.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			applyCategory(plan, client.Unwrap(out, "category"))
			warnContrast(d, out)
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *categoryModel, d *diag.Diagnostics) error {
			q := url.Values{}
			if known(m.DeleteMoveTopicsTo) {
				q.Set("moveTo", m.DeleteMoveTopicsTo.ValueString())
			}
			if m.DeleteForce.ValueBool() {
				q.Set("force", "true")
			}
			p := c.SpacePath(m.SpaceID.ValueString(), "forum", "categories", m.ID.ValueString())
			if len(q) > 0 {
				p += "?" + q.Encode()
			}
			return c.Do(ctx, http.MethodDelete, p, nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 2)
			if err != nil {
				return nil, err
			}
			return map[string]string{"space_id": p[0], "id": p[1]}, nil
		},
		validate: func(ctx context.Context, cfg *categoryModel, d *diag.Diagnostics) {
			if known(cfg.DeleteMoveTopicsTo) && cfg.DeleteForce.ValueBool() {
				d.AddAttributeError(attrPath("delete_force"), "Conflicting destroy options", "Set either delete_move_topics_to or delete_force, not both.")
			}
		},
	})
}

func tagGroupsBody(l types.List) []any {
	out := []any{}
	for _, e := range l.Elements() {
		o, ok := e.(types.Object)
		if !ok {
			continue
		}
		a := o.Attributes()
		g, _ := a["tag_group_id"].(types.String)
		n, _ := a["min_count"].(types.Int64)
		out = append(out, map[string]any{"tagGroupId": g.ValueString(), "minCount": n.ValueInt64()})
	}
	return out
}

func tagGroupsFromAPI(raw any) types.List {
	arr, _ := raw.([]any)
	vals := make([]attr.Value, 0, len(arr))
	for _, e := range arr {
		o, _ := e.(map[string]any)
		obj, _ := types.ObjectValue(tagGroupAttrTypes, map[string]attr.Value{"tag_group_id": getS(o, "tagGroupId"), "min_count": getI(o, "minCount")})
		vals = append(vals, obj)
	}
	l, _ := types.ListValue(types.ObjectType{AttrTypes: tagGroupAttrTypes}, vals)
	return l
}

// warnContrast surfaces the API's WCAG warnings on the colour pairs.
func warnContrast(d *diag.Diagnostics, out Object) {
	arr, _ := out["warnings"].([]any)
	for _, w := range arr {
		if o, ok := w.(map[string]any); ok {
			if msg, ok := o["message"].(string); ok {
				d.AddWarning("Forumulate API warning", msg)
			}
		}
	}
}

func applyCategory(m *categoryModel, row Object) {
	readFields(m, categoryFields, row, map[string]bool{"color": true, "color_dark": true, "text_color": true, "text_color_dark": true})
	m.ID = getS(row, "id")
	m.TopicCount = getI(row, "topicCount")
	m.TopicTemplateJSON = jsonFromAPI(m.TopicTemplateJSON, row["topicTemplate"])
	m.RequiredTagGroups = tagGroupsFromAPI(row["requiredTagGroups"])
	m.Translations = translationsFromAPI(m.Translations, row["translations"])
	// The API stores an unset description as ""; a configured "" stays "".
	if m.Description.IsNull() {
		m.Description = types.StringValue("")
	}
}
