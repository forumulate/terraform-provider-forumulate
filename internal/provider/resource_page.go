package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pageModel struct {
	ID              types.String `tfsdk:"id"`
	SpaceID         types.String `tfsdk:"space_id"`
	CollectionID    types.String `tfsdk:"collection_id"`
	Slug            types.String `tfsdk:"slug"`
	Title           types.String `tfsdk:"title"`
	Body            types.String `tfsdk:"body"`
	ParentID        types.String `tfsdk:"parent_id"`
	AfterID         types.String `tfsdk:"after_id"`
	Preset          types.String `tfsdk:"preset"`
	Locale          types.String `tfsdk:"locale"`
	VersionKey      types.String `tfsdk:"version_key"`
	Published       types.Bool   `tfsdk:"published"`
	Metadata        types.Object `tfsdk:"metadata"`
	MetadataJSON    types.String `tfsdk:"metadata_json"`
	DestroyOnDelete types.Bool   `tfsdk:"destroy_on_delete"`
	BodyHash        types.String `tfsdk:"body_hash"`
	ContentHash     types.String `tfsdk:"content_hash"`
	URL             types.String `tfsdk:"url"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

// pageMetaFields are the page metadata keys with a typed attribute; every
// other key goes through metadata_json.
var pageMetaFields = []fieldMap{
	{"tags", "tags", kStrSet},
	{"icon", "icon", kStr},
	{"hide_from_nav", "hideFromNav", kBool},
	{"search_exclude", "searchExclude", kBool},
	{"nav_title", "navTitle", kStr},
	{"cover_url", "coverUrl", kStr},
	{"seo_title", "seoTitle", kStr},
	{"seo_description", "seoDescription", kStr},
	{"canonical_url", "canonicalUrl", kStr},
	{"noindex", "noindex", kBool},
}

var kindRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
var versionKeyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var pageLocaleRE = regexp.MustCompile(`^(default|[a-z]{2,3}(-[a-z0-9]{2,8})*)$`)

// NewPageResource is `forumulate_page`.
func NewPageResource() resource.Resource {
	return newResource(&resourceSpec[pageModel]{
		suffix: "page",
		what:   "page",
		description: "A page of a docs-style space, written in Markdown. The body is sent through the API's Markdown support and converted to the same blocks the editor makes; " +
			"the resource then publishes it (or leaves it a draft). Its place in the tree (`collection_id`, `parent_id`, `after_id`), its kind (`preset`) and its metadata (SEO, navigation, tags) are managed too.\n\n" +
			"Drift is detected in the body (compared by the API's own normalised Markdown, so a list marker the API rewrites is not a diff), the title, the slug, the place in the tree, the publish state and the metadata keys you declare. " +
			"Destroying a page moves it to the trash; `destroy_on_delete` makes that permanent.",
		importHelp: "<space_id>/<page_id>",
		paramAlias: map[string]string{"parentId": "parent_id", "afterId": "after_id", "collectionId": "collection_id", "versionKey": "version_key", "seoTitle": "metadata", "seoDescription": "metadata", "content": "body"},
		attrs: map[string]schema.Attribute{
			"id":            computedID("The page's id (a UUID)."),
			"space_id":      reqStr("The space (its id or slug). Changing it replaces the page.", replStr, idOrSlug()),
			"collection_id": reqStr("The collection the page lives in (its id). Changing it moves the page.", nil, uuid()),
			"slug":          reqStr("The page's address segment (1 to 80 characters, lowercase letters, digits and hyphens). A slug already taken in the collection is an error, never a silent suffix.", nil, slug(1, 80)...),
			"title":         reqStr("The page's title, 1 to 300 characters.", nil, length(1, 300)),
			"body": optStr("The page's Markdown (up to 2,000,000 characters), without the leading `# Title` line. See the Markdown dialect in the API docs for directives and snippets.",
				length(0, 2000000)),
			"parent_id": optStr("The page this one nests under (its id, in the same collection); leave out for top level.", uuid()),
			"after_id":  optStr("Place the page directly after this sibling (its id). Changing it reorders; the API does not return a page's order, so drift is not detected.", uuid()),
			"preset": schema.StringAttribute{
				Optional: true, Validators: []validator.String{pattern(kindRE, "must be a kind of piece the space has, lowercase letters, digits and hyphens (up to 40 characters).")},
				MarkdownDescription: "The kind of piece this page is (one the space declares; `GET /space-types/{type}`). Changing it is validated by the API against the kind's shape.",
			},
			"locale": schema.StringAttribute{
				Optional: true, Computed: true, Validators: []validator.String{pattern(pageLocaleRE, "must be `default` or a language code like es or pt-br.")},
				PlanModifiers:       append(append([]planmodifier.String{}, keepStr...), replStr...),
				MarkdownDescription: "The page's language: `default` (the space's source language) or a code the space declares. Changing it replaces the page.",
			},
			"version_key": schema.StringAttribute{
				Optional: true, Computed: true, Validators: []validator.String{length(1, 40), pattern(versionKeyRE, "must start with a lowercase letter or digit and use lowercase letters, digits and . _ -")},
				PlanModifiers:       append(append([]planmodifier.String{}, keepStr...), replStr...),
				MarkdownDescription: "The docs version the page belongs to (up to 40 characters). Defaults to the current version. Changing it replaces the page.",
			},
			"published": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether readers see the page. `true` (the default) publishes after every change; `false` keeps it a draft and unpublishes a live page.",
			},
			"metadata": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "The common page metadata. Only the attributes you set are managed; removing one clears it. Everything else is `metadata_json`.",
				Attributes: map[string]schema.Attribute{
					"tags":            schema.SetAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Set{setOf(0, 12, length(1, 60))}, MarkdownDescription: "Up to 12 tags, 60 characters each."},
					"icon":            schema.StringAttribute{Optional: true, Validators: []validator.String{iconName(collectionIcons)}, MarkdownDescription: "A named icon or a custom icon (path, URL, emoji)."},
					"hide_from_nav":   schema.BoolAttribute{Optional: true, MarkdownDescription: "Leave the page out of the sidebar."},
					"search_exclude":  schema.BoolAttribute{Optional: true, MarkdownDescription: "Leave the page out of search."},
					"nav_title":       schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 80)}, MarkdownDescription: "A shorter title for the sidebar (up to 80 characters)."},
					"cover_url":       schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 2048)}, MarkdownDescription: "A cover image: an uploaded path or an http(s) URL."},
					"seo_title":       schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 200)}, MarkdownDescription: "The `<title>` (up to 200 characters)."},
					"seo_description": schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 400)}, MarkdownDescription: "The meta description (up to 400 characters)."},
					"canonical_url":   schema.StringAttribute{Optional: true, Validators: []validator.String{httpURL(2048, false)}, MarkdownDescription: "The canonical URL when it is not the page's own address."},
					"noindex":         schema.BoolAttribute{Optional: true, MarkdownDescription: "Ask search engines not to index the page."},
				},
			},
			"metadata_json":     jsonAttr("Page metadata beyond the `metadata` attribute: `fields` (the space type's declared fields), `audience`, `layout`, `hero`, `badge`, `template`, `themePatch`, and the rest of `PATCH …/pages/{id}/metadata`. Keys are camelCase, as the API spells them.", pageMetadataKeys...),
			"destroy_on_delete": schema.BoolAttribute{Optional: true, MarkdownDescription: "On destroy, permanently delete the page after trashing it, freeing its slug. Defaults to `false`: the page stays in the trash, restorable."},
			"body_hash":         schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 of the API's normalised Markdown for the page, used to tell a real edit made in the studio from the API's rewriting of your Markdown."},
			"content_hash":      schema.StringAttribute{Computed: true, MarkdownDescription: "The API's `contentHash` of the published body."},
			"url":               schema.StringAttribute{Computed: true, MarkdownDescription: "The page's public address, when it has one."},
			"updated_at":        schema.StringAttribute{Computed: true, MarkdownDescription: "When the page last changed."},
		},
		create:  createPage,
		created: func(m *pageModel) bool { return known(m.ID) },
		read:    readPage,
		update:  updatePage,
		del: func(ctx context.Context, c *client.Client, m *pageModel, d *diag.Diagnostics) error {
			p := c.SpacePath(m.SpaceID.ValueString(), "pages", m.ID.ValueString())
			if err := c.Do(ctx, http.MethodDelete, p, nil, nil); err != nil {
				return err
			}
			if m.DestroyOnDelete.ValueBool() {
				return c.Do(ctx, http.MethodDelete, p+"/destroy?"+url.Values{"confirm": {m.Slug.ValueString()}}.Encode(), nil, nil)
			}
			return nil
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 2)
			if err != nil {
				return nil, err
			}
			return map[string]string{"space_id": p[0], "id": p[1]}, nil
		},
		validate: func(ctx context.Context, cfg *pageModel, d *diag.Diagnostics) {
			if known(cfg.ParentID) && known(cfg.AfterID) && cfg.ParentID.Equal(cfg.AfterID) {
				d.AddAttributeError(attrPath("after_id"), "Invalid placement", "A page cannot be placed after its own parent in the same step; after_id must be a sibling.")
			}
		},
	})
}

func pagePath(c *client.Client, m *pageModel, rest ...string) string {
	return c.SpacePath(m.SpaceID.ValueString(), append([]string{"pages", m.ID.ValueString()}, rest...)...)
}

func createPage(ctx context.Context, c *client.Client, m *pageModel, d *diag.Diagnostics) error {
	body := Object{"title": m.Title.ValueString(), "slug": m.Slug.ValueString(), "onSlugTaken": "fail"}
	setStr(body, "parentId", m.ParentID, false)
	setStr(body, "preset", m.Preset, false)
	setStr(body, "locale", m.Locale, false)
	setStr(body, "versionKey", m.VersionKey, false)
	if known(m.Body) && m.Body.ValueString() != "" {
		body["body"] = m.Body.ValueString()
	}
	out, err := c.DoObject(ctx, http.MethodPost, c.SpacePath(m.SpaceID.ValueString(), "collections", m.CollectionID.ValueString(), "pages"), body)
	if err != nil {
		return err
	}
	warnList(d, out)
	m.ID = getS(client.Unwrap(out, "page"), "id")
	pm := *m
	if known(m.AfterID) {
		if err := c.Do(ctx, http.MethodPost, pagePath(c, &pm, "reorder"), Object{"afterId": m.AfterID.ValueString()}, nil); err != nil {
			return err
		}
	}
	if err := writeMetadata(ctx, c, m, nil, d); err != nil {
		return err
	}
	if m.Published.ValueBool() {
		if err := publishPage(ctx, c, &pm, d); err != nil {
			return err
		}
	}
	return refreshPage(ctx, c, m, true, d)
}

func readPage(ctx context.Context, c *client.Client, m *pageModel, d *diag.Diagnostics) error {
	return refreshPage(ctx, c, m, false, d)
}

func updatePage(ctx context.Context, c *client.Client, plan, prior *pageModel, d *diag.Diagnostics) error {
	plan.ID = prior.ID
	// Rename and re-slug.
	if !plan.Title.Equal(prior.Title) || !plan.Slug.Equal(prior.Slug) {
		body := Object{"title": plan.Title.ValueString(), "slug": plan.Slug.ValueString(), "onSlugTaken": "fail"}
		out, err := c.DoObject(ctx, http.MethodPatch, pagePath(c, plan), body)
		if err != nil {
			return err
		}
		warnList(d, out)
	}
	// Body.
	bodyChanged := !plan.Body.Equal(prior.Body)
	if bodyChanged {
		body := Object{"body": plan.Body.ValueString()}
		if plan.Body.IsNull() {
			body["body"] = ""
		}
		out, err := c.DoObject(ctx, http.MethodPut, pagePath(c, plan, "content"), body)
		if err != nil {
			return err
		}
		warnList(d, out)
	}
	// Place in the tree.
	if !plan.CollectionID.Equal(prior.CollectionID) || !plan.ParentID.Equal(prior.ParentID) || !plan.AfterID.Equal(prior.AfterID) {
		body := Object{"collectionId": plan.CollectionID.ValueString()}
		if known(plan.ParentID) {
			body["parentId"] = plan.ParentID.ValueString()
		} else {
			body["parentId"] = nil
		}
		if known(plan.AfterID) {
			body["afterId"] = plan.AfterID.ValueString()
		}
		out, err := c.DoObject(ctx, http.MethodPost, pagePath(c, plan, "reorder"), body)
		if err != nil {
			return err
		}
		warnList(d, out)
	}
	// Kind.
	if !plan.Preset.Equal(prior.Preset) {
		body := Object{"preset": nil}
		if known(plan.Preset) {
			body["preset"] = plan.Preset.ValueString()
		}
		if err := c.Do(ctx, http.MethodPut, pagePath(c, plan, "shape"), body, nil); err != nil {
			return err
		}
	}
	if err := writeMetadata(ctx, c, plan, prior, d); err != nil {
		return err
	}
	// Publish state. A live page whose body changed is published again.
	switch {
	case plan.Published.ValueBool() && (bodyChanged || !prior.Published.ValueBool() || !plan.Title.Equal(prior.Title)):
		if err := publishPage(ctx, c, plan, d); err != nil {
			return err
		}
	case !plan.Published.ValueBool() && prior.Published.ValueBool():
		if err := c.Do(ctx, http.MethodPost, pagePath(c, plan, "unpublish"), nil, nil); err != nil {
			return err
		}
	}
	return refreshPage(ctx, c, plan, true, d)
}

func publishPage(ctx context.Context, c *client.Client, m *pageModel, d *diag.Diagnostics) error {
	out, err := c.DoObject(ctx, http.MethodPost, pagePath(c, m, "publish"), Object{})
	if err != nil {
		return err
	}
	warnList(d, out)
	return nil
}

// writeMetadata PATCHes the metadata that changed: typed attributes by field,
// metadata_json by key, with nulls for what the configuration dropped.
func writeMetadata(ctx context.Context, c *client.Client, m, prior *pageModel, d *diag.Diagnostics) error {
	var priorObj *types.Object
	var priorJSON types.String
	if prior != nil {
		priorObj = &prior.Metadata
		priorJSON = prior.MetadataJSON
		if m.Metadata.Equal(prior.Metadata) && m.MetadataJSON.Equal(prior.MetadataJSON) {
			return nil
		}
	}
	body := Object{}
	if known(m.Metadata) || (priorObj != nil && known(*priorObj)) {
		for k, v := range objectBody(m.Metadata, pageMetaFields, priorObj) {
			body[k] = v
		}
	}
	extra := Object{}
	mergeBody(extra, "x", priorJSON, m.MetadataJSON)
	if x, ok := extra["x"].(map[string]any); ok {
		for k, v := range x {
			body[k] = v
		}
	}
	if len(body) == 0 {
		return nil
	}
	out, err := c.DoObject(ctx, http.MethodPatch, pagePath(c, m, "metadata"), body)
	if err != nil {
		return err
	}
	warnList(d, out)
	return nil
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// refreshPage reads the page into the model. After a write (`wrote`) the
// configured body is kept and only the normalised hash is recorded; on a plain
// read a body that no longer matches the recorded hash is drift.
func refreshPage(ctx context.Context, c *client.Client, m *pageModel, wrote bool, d *diag.Diagnostics) error {
	out, err := c.DoObject(ctx, http.MethodGet, pagePath(c, m), nil)
	if err != nil {
		return err
	}
	pg := client.Unwrap(out, "page")
	m.ID = getS(pg, "id")
	m.Slug = getS(pg, "slug")
	m.Title = getS(pg, "title")
	m.Locale = getS(pg, "locale")
	m.VersionKey = getS(pg, "versionKey")
	m.ContentHash = getS(pg, "contentHash")
	m.URL = getS(pg, "url")
	m.UpdatedAt = getS(pg, "updatedAt")
	if col, ok := pg["collection"].(map[string]any); ok {
		m.CollectionID = getS(col, "id")
	}
	m.ParentID = getS(pg, "parentId")
	m.Published = types.BoolValue(strings.EqualFold(stringOf(pg["status"]), "published"))
	// The metadata door is the one that is documented to hold every key.
	meta := map[string]any{}
	if known(m.Metadata) || known(m.MetadataJSON) {
		mo, err := c.DoObject(ctx, http.MethodGet, pagePath(c, m, "metadata"), nil)
		if err != nil {
			return err
		}
		meta, _ = mo["metadata"].(map[string]any)
	}
	m.Metadata = objectFromAPI(m.Metadata, pageMetaFields, meta, false)
	m.MetadataJSON = jsonFromAPI(m.MetadataJSON, anyOrNil(meta))

	remote, err := c.DoText(ctx, pagePath(c, m, "draft", "markdown")+"?form=body")
	if err != nil {
		return err
	}
	norm := strings.TrimSpace(remote)
	h := sha(norm)
	switch {
	case wrote:
		m.BodyHash = types.StringValue(h)
	case m.BodyHash.IsNull() || m.BodyHash.IsUnknown() || m.BodyHash.ValueString() != h:
		// Changed since Terraform last wrote it (or first read, on import).
		if norm == "" {
			m.Body = types.StringNull()
		} else {
			m.Body = types.StringValue(norm)
		}
		m.BodyHash = types.StringValue(h)
	}
	if m.Body.IsUnknown() {
		m.Body = types.StringNull()
	}
	return nil
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}
