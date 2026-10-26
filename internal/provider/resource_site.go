package provider

import (
	"context"
	"net/http"
	"regexp"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type siteModel struct {
	ID               types.String `tfsdk:"id"`
	Slug             types.String `tfsdk:"slug"`
	Name             types.String `tfsdk:"name"`
	SpaceIDs         types.Set    `tfsdk:"space_ids"`
	ConfigJSON       types.String `tfsdk:"config_json"`
	PageMounts       types.List   `tfsdk:"page_mounts"`
	TranslationsJSON types.String `tfsdk:"translations_json"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ExternalRef      types.String `tfsdk:"external_ref"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

var pageMountTypes = map[string]attr.Type{"path": types.StringType, "space_id": types.StringType, "page": types.StringType}

var pageMountPathRE = regexp.MustCompile(`^/(?:[a-z0-9]+(?:-[a-z0-9]+)*)?$`)

// siteConfigKeysNoMounts are the config keys config_json may carry: page
// mounts have their own typed attribute.
func siteConfigKeysNoMounts() []string {
	out := []string{}
	for _, k := range siteConfigKeys {
		if k != "pageMounts" {
			out = append(out, k)
		}
	}
	return out
}

// NewSiteResource is `forumulate_site`.
func NewSiteResource() resource.Resource {
	return newResource(&resourceSpec[siteModel]{
		suffix: "site",
		what:   "site",
		description: "A site gathers public spaces under one address so they share a header, a footer and a theme. `space_ids` is its membership, `config_json` its shared chrome and look (header and footer links, theme, search, SEO, analytics, ...), " +
			"and `page_mounts` publishes single pages of member spaces at a path of the site (`/pricing`) or at its root.\n\n" +
			"Destroying a site dissolves the grouping; every space in it survives with its own content and theme.",
		importHelp: "the site's id or slug",
		paramAlias: map[string]string{"config": "config_json", "spaceIds": "space_ids", "pageMounts": "page_mounts", "translations": "translations_json", "externalRef": "external_ref"},
		attrs: map[string]schema.Attribute{
			"id":   computedID("The site's id (a UUID)."),
			"slug": reqStr("The site's slug (2 to 48 characters, lowercase letters, digits and hyphens). Changing it replaces the site.", replStr, slug(2, 48)...),
			"name": reqStr("The site's name, 1 to 160 characters.", nil, length(1, 160)),
			"space_ids": schema.SetAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Set{setOf(0, 100, uuid())},
				MarkdownDescription: "The member spaces (their ids, up to 100). When set, the site's membership is exactly this list. Leave it out to leave membership alone: later public spaces join the oldest site by themselves."},
			"config_json": jsonAttr("The site's config, deep-merged into the stored one: nested objects merge, lists and scalars replace, and a key you remove from the configuration is cleared. "+
				"Keys: header/footer chrome (`cta`, `header`, `headerLinks`, `footerLinks`, `footerColumns`, `navMenu`, `mobileNav`, ...), `theme`, `search`, `searchSettings`, `seo`, `analytics`, `comments`, `discovery`, `ai`, ... Page mounts are `page_mounts`.", siteConfigKeysNoMounts()...),
			"page_mounts": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Single pages published at a path of the site (up to 50). The space must be a member, the page must exist, and a path may not shadow a member space's own segment.",
				Validators:          []validator.List{listOf(0, 50)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"path":     schema.StringAttribute{Required: true, Validators: []validator.String{pattern(pageMountPathRE, "must be / (the site root) or one segment like /pricing.")}, MarkdownDescription: "Where the page answers: `/` for the root, or one segment such as `/pricing`."},
					"space_id": schema.StringAttribute{Required: true, Validators: []validator.String{uuid()}, MarkdownDescription: "The member space the page belongs to (its id)."},
					"page":     schema.StringAttribute{Required: true, Validators: []validator.String{length(1, 200)}, MarkdownDescription: "The page's slug in that space."},
				}},
			},
			"translations_json": jsonAttr("The site's name and description in other languages, per language code (`name`, `description`, `seoTitle`, `seoDescription`, `pages`). Replaced whole when changed."),
			"metadata": schema.MapAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Map{metadataMap()},
				MarkdownDescription: "Free-form tags. Replaced whole when changed. The API does not return them on a read, so drift is not detected."},
			"external_ref": schema.StringAttribute{Optional: true, Validators: externalRef(), PlanModifiers: replStr,
				MarkdownDescription: "Your own reference, fixed at creation."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "When the site was created."},
		},
		create: func(ctx context.Context, c *client.Client, m *siteModel, d *diag.Diagnostics) error {
			body := Object{"slug": m.Slug.ValueString(), "name": m.Name.ValueString()}
			if known(m.SpaceIDs) {
				body["spaceIds"] = elementsStrings(m.SpaceIDs)
			}
			if cfg := siteConfigBody(types.StringNull(), m.ConfigJSON, types.ListNull(types.ObjectType{AttrTypes: pageMountTypes}), m.PageMounts); cfg != nil {
				body["config"] = cfg
			}
			setJSON(body, "translations", m.TranslationsJSON, false)
			setStr(body, "externalRef", m.ExternalRef, false)
			setStrMap(body, "metadata", m.Metadata, false)
			out, err := c.DoObject(ctx, http.MethodPost, c.OrgPath("sites"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			warnContrastSite(d, out)
			st := client.Unwrap(out, "site")
			m.ID = getS(st, "id")
			applySite(m, st)
			return nil
		},
		created: func(m *siteModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *siteModel, d *diag.Diagnostics) error {
			ref := m.ID.ValueString()
			if ref == "" {
				ref = m.Slug.ValueString()
			}
			out, err := c.DoObject(ctx, http.MethodGet, c.SitePath(ref), nil)
			if err != nil {
				return err
			}
			applySite(m, client.Unwrap(out, "site"))
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *siteModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			body := Object{"name": plan.Name.ValueString()}
			if known(plan.SpaceIDs) && !plan.SpaceIDs.Equal(prior.SpaceIDs) {
				body["spaceIds"] = elementsStrings(plan.SpaceIDs)
			}
			if !plan.ConfigJSON.Equal(prior.ConfigJSON) || !plan.PageMounts.Equal(prior.PageMounts) {
				if cfg := siteConfigBody(prior.ConfigJSON, plan.ConfigJSON, prior.PageMounts, plan.PageMounts); cfg != nil {
					body["config"] = cfg
				}
			}
			if !plan.TranslationsJSON.Equal(prior.TranslationsJSON) {
				setJSON(body, "translations", plan.TranslationsJSON, true)
			}
			if !plan.Metadata.Equal(prior.Metadata) {
				setStrMap(body, "metadata", plan.Metadata, true)
			}
			out, err := c.DoObject(ctx, http.MethodPatch, c.SitePath(prior.ID.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			warnContrastSite(d, out)
			applySite(plan, client.Unwrap(out, "site"))
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *siteModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.SitePath(m.ID.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) { return map[string]string{"id": id}, nil },
		validate: func(ctx context.Context, cfg *siteModel, d *diag.Diagnostics) {
			if !known(cfg.PageMounts) {
				return
			}
			seen := map[string]bool{}
			for _, e := range cfg.PageMounts.Elements() {
				o, ok := e.(types.Object)
				if !ok || o.IsUnknown() {
					continue
				}
				p, _ := o.Attributes()["path"].(types.String)
				if known(p) {
					if seen[p.ValueString()] {
						d.AddAttributeError(attrPath("page_mounts"), "Duplicate mount", "Two page mounts use the path "+p.ValueString()+"; a path holds one page.")
					}
					seen[p.ValueString()] = true
				}
			}
		},
	})
}

// siteConfigBody builds the `config` of a create/patch: the config_json
// document (with nulls for removed keys) plus the page mounts list.
func siteConfigBody(priorJSON, planJSON types.String, priorMounts, planMounts types.List) Object {
	holder := Object{}
	mergeBody(holder, "c", priorJSON, planJSON)
	cfg, _ := holder["c"].(map[string]any)
	if cfg == nil {
		cfg = map[string]any{}
	}
	switch {
	case known(planMounts):
		list := []any{}
		for _, e := range planMounts.Elements() {
			o, ok := e.(types.Object)
			if !ok {
				continue
			}
			a := o.Attributes()
			path, _ := a["path"].(types.String)
			sp, _ := a["space_id"].(types.String)
			pg, _ := a["page"].(types.String)
			list = append(list, map[string]any{"path": path.ValueString(), "spaceId": sp.ValueString(), "page": pg.ValueString()})
		}
		cfg["pageMounts"] = list
	case known(priorMounts):
		cfg["pageMounts"] = []any{}
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

func warnContrastSite(d *diag.Diagnostics, out Object) {
	arr, _ := out["contrastWarnings"].([]any)
	for _, w := range arr {
		if o, ok := w.(map[string]any); ok {
			if msg, ok := o["message"].(string); ok {
				d.AddWarning("Forumulate API warning", msg)
			}
		}
	}
}

func applySite(m *siteModel, st Object) {
	m.ID = getS(st, "id")
	m.Slug = getS(st, "slug")
	m.Name = getS(st, "name")
	m.CreatedAt = getS(st, "createdAt")
	if known(m.SpaceIDs) {
		m.SpaceIDs = getSS(st, "spaceIds")
	}
	cfg, _ := st["config"].(map[string]any)
	// Page mounts have their own attribute: take them out of the comparison.
	var mounts any
	if cfg != nil {
		mounts = cfg["pageMounts"]
		rest := map[string]any{}
		for k, v := range cfg {
			if k != "pageMounts" {
				rest[k] = v
			}
		}
		cfg = rest
	}
	m.ConfigJSON = jsonFromAPI(m.ConfigJSON, anyOrNil(cfg))
	if known(m.PageMounts) {
		arr, _ := mounts.([]any)
		vals := make([]attr.Value, 0, len(arr))
		for _, e := range arr {
			o, _ := e.(map[string]any)
			obj, _ := types.ObjectValue(pageMountTypes, map[string]attr.Value{"path": getS(o, "path"), "space_id": getS(o, "spaceId"), "page": getS(o, "page")})
			vals = append(vals, obj)
		}
		m.PageMounts, _ = types.ListValue(types.ObjectType{AttrTypes: pageMountTypes}, vals)
	}
	m.TranslationsJSON = jsonFromAPI(m.TranslationsJSON, st["translations"])
	if m.Metadata.IsUnknown() {
		m.Metadata = types.MapNull(types.StringType)
	}
}
