package provider

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var customIconRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]+$`)

// iconName accepts a named icon from the list, or a value that does not look
// like a name at all (an emoji, a path, a URL), which the API takes as a
// custom icon.
func iconName(names []string) validator.String {
	return stringCheck{
		desc: "a named icon or a custom icon",
		fn: func(s string) string {
			if !customIconRE.MatchString(s) {
				return ""
			}
			for _, n := range names {
				if n == s {
					return ""
				}
			}
			return "\"" + s + "\" is not a known icon name. Use one of the named icons (see the resource docs) or a custom icon (a path, URL or emoji)."
		},
	}
}

type collectionModel struct {
	ID           types.String `tfsdk:"id"`
	SpaceID      types.String `tfsdk:"space_id"`
	Name         types.String `tfsdk:"name"`
	Slug         types.String `tfsdk:"slug"`
	Description  types.String `tfsdk:"description"`
	CoverURL     types.String `tfsdk:"cover_url"`
	ParentID     types.String `tfsdk:"parent_id"`
	Nav          types.Object `tfsdk:"nav"`
	SEO          types.Object `tfsdk:"seo"`
	Translations types.Map    `tfsdk:"translations"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
}

var collectionNavFields = []fieldMap{
	{"collapsed", "collapsed", kBool},
	{"icon", "icon", kStr},
	{"link_hub", "linkHub", kBool},
	{"hub_cards", "hubCards", kBool},
	{"nav_scope", "navScope", kStr},
	{"back_style", "backStyle", kStr},
	{"audience_principals", "audience.principals", kStrSet},
	{"audience_roles", "audience.roles", kStrSet},
}

var collectionSEOFields = []fieldMap{
	{"title", "title", kStr},
	{"description", "description", kStr},
	{"index", "index", kStr},
}

var audienceItem = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,59}$`)

// NewCollectionResource is `forumulate_collection`.
func NewCollectionResource() resource.Resource {
	audience := func(what string) schema.SetAttribute {
		return schema.SetAttribute{
			Optional: true, ElementType: types.StringType,
			Validators:          []validator.Set{setOf(0, 20, pattern(audienceItem, "must be 1 to 60 characters: lowercase letters, digits and _ . : -"))},
			MarkdownDescription: "Limit the collection to readers with one of these " + what + " (up to 20).",
		}
	}
	return newResource(&resourceSpec[collectionModel]{
		suffix: "collection",
		what:   "collection",
		description: "A collection: the container a docs space groups its pages into, and the navigation tree readers see. " +
			"Collections nest (`parent_id`), carry a landing page (`description`, `cover_url`, `seo`) and decide how the sidebar treats them (`nav`).",
		importHelp: "<space_id>/<collection_id>",
		paramAlias: map[string]string{"coverUrl": "cover_url", "parentId": "parent_id"},
		attrs: map[string]schema.Attribute{
			"id":       computedID("The collection's id (a UUID)."),
			"space_id": reqStr("The docs space (its id or slug). Changing it replaces the collection.", replStr, idOrSlug()),
			"name":     reqStr("The collection's name, 1 to 160 characters.", nil, length(1, 160)),
			"slug": optComputedStr("The collection's address segment (1 to 80 characters, lowercase letters, digits and hyphens). Derived from the name when left out.",
				slug(1, 80)...),
			"description": optStr("The landing page's subtitle and meta description, up to 500 characters.", length(0, 500)),
			"cover_url":   optStr("A banner above the landing page: an uploaded path or an http(s) URL, up to 2048 characters.", length(0, 2048)),
			"parent_id":   optStr("The parent collection's id; leave out for a top-level collection.", uuid()),
			"nav": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "How the reader's sidebar treats this collection. Only the attributes you set are managed; removing one returns it to the theme's default.",
				Attributes: map[string]schema.Attribute{
					"collapsed":           schema.BoolAttribute{Optional: true, MarkdownDescription: "Start the group collapsed."},
					"icon":                schema.StringAttribute{Optional: true, Validators: []validator.String{iconName(collectionIcons)}, MarkdownDescription: "A named icon, or a custom icon (path, URL or emoji)."},
					"link_hub":            schema.BoolAttribute{Optional: true, MarkdownDescription: "Make the group's heading link to its landing page."},
					"hub_cards":           schema.BoolAttribute{Optional: true, MarkdownDescription: "Draw the landing page as cards."},
					"nav_scope":           schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("shared", "isolated")}, MarkdownDescription: "`shared`, or `isolated` to show only this collection's pages in the sidebar."},
					"back_style":          schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("link", "title")}, MarkdownDescription: "`link` or `title`: the way back out of an isolated collection."},
					"audience_principals": audience("principals"),
					"audience_roles":      audience("roles"),
				},
			},
			"seo": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Search-result overrides for the landing page. Replaced whole when changed.",
				Attributes: map[string]schema.Attribute{
					"title":       schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 120)}, MarkdownDescription: "The `<title>`, up to 120 characters."},
					"description": schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 320)}, MarkdownDescription: "The meta description, up to 320 characters."},
					"index":       schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("auto", "index", "noindex")}, MarkdownDescription: "`auto` (indexed unless thin), `index` or `noindex`."},
				},
			},
			"translations": nameDescTranslations("The collection's name and description in other languages."),
			"is_default":   schema.BoolAttribute{Computed: true, PlanModifiers: keepBln, MarkdownDescription: "Whether this is the space's default collection."},
		},
		create: func(ctx context.Context, c *client.Client, m *collectionModel, d *diag.Diagnostics) error {
			body := Object{"name": m.Name.ValueString()}
			setStr(body, "slug", m.Slug, false)
			setStr(body, "description", m.Description, false)
			setStr(body, "coverUrl", m.CoverURL, false)
			setStr(body, "parentId", m.ParentID, false)
			if known(m.Nav) {
				body["nav"] = objectBody(m.Nav, collectionNavFields, nil)
			}
			if known(m.SEO) {
				body["seo"] = objectBody(m.SEO, collectionSEOFields, nil)
			}
			if known(m.Translations) {
				body["translations"] = translationsBody(m.Translations)
			}
			out, err := c.DoObject(ctx, http.MethodPost, c.SpacePath(m.SpaceID.ValueString(), "collections"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			applyCollection(m, client.Unwrap(out, "collection"))
			return nil
		},
		created: func(m *collectionModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *collectionModel, d *diag.Diagnostics) error {
			row, err := findRow(ctx, c, c.SpacePath(m.SpaceID.ValueString(), "collections"), "collections", "id", m.ID.ValueString(), "collection")
			if err != nil {
				return err
			}
			applyCollection(m, row)
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *collectionModel, d *diag.Diagnostics) error {
			body := Object{"name": plan.Name.ValueString()}
			setStr(body, "slug", plan.Slug, false)
			setStr(body, "description", plan.Description, true)
			setStr(body, "coverUrl", plan.CoverURL, true)
			setStr(body, "parentId", plan.ParentID, true)
			if known(plan.Nav) || known(prior.Nav) {
				body["nav"] = objectBody(plan.Nav, collectionNavFields, &prior.Nav)
			}
			if known(plan.SEO) || known(prior.SEO) {
				body["seo"] = objectBody(plan.SEO, collectionSEOFields, nil)
			}
			if known(plan.Translations) || known(prior.Translations) {
				body["translations"] = translationsBody(plan.Translations)
			}
			plan.ID = prior.ID
			out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(plan.SpaceID.ValueString(), "collections", prior.ID.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			applyCollection(plan, client.Unwrap(out, "collection"))
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *collectionModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.SpacePath(m.SpaceID.ValueString(), "collections", m.ID.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 2)
			if err != nil {
				return nil, err
			}
			return map[string]string{"space_id": p[0], "id": p[1]}, nil
		},
	})
}

func applyCollection(m *collectionModel, row Object) {
	m.ID = getS(row, "id")
	m.Name = getS(row, "name")
	m.Slug = getS(row, "slug")
	m.Description = nullIfEmpty(getS(row, "description"), m.Description)
	m.CoverURL = nullIfEmpty(getS(row, "coverUrl"), m.CoverURL)
	m.ParentID = getS(row, "parentId")
	m.IsDefault = getB(row, "isDefault")
	nav, _ := row["nav"].(map[string]any)
	m.Nav = objectFromAPI(m.Nav, collectionNavFields, nav, false)
	seo, _ := row["seo"].(map[string]any)
	m.SEO = objectFromAPI(m.SEO, collectionSEOFields, seo, false)
	m.Translations = translationsFromAPI(m.Translations, row["translations"])
}

// nullIfEmpty treats "" from the API as the null the configuration holds when
// the user left an optional field out, and keeps a configured "" as "".
func nullIfEmpty(api, prior types.String) types.String {
	if api.IsNull() || api.ValueString() == "" {
		if known(prior) && prior.ValueString() == "" {
			return prior
		}
		return types.StringNull()
	}
	return api
}

// findRow finds one row of a list by a key. There is no single-GET for
// collections, categories, webhooks and similar, so Read filters the list; a
// missing row is the 404 that removes the resource from state.
func findRow(ctx context.Context, c *client.Client, path, key, field, want, what string) (Object, error) {
	rows, err := c.List(ctx, path, key, nil)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if v, _ := r[field].(string); v == want {
			return r, nil
		}
	}
	return nil, client.NotFound(what + " " + strings.TrimSpace(want))
}
