package provider

import (
	"context"
	"net/http"
	"net/url"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// SpaceTypes are the kinds of space the API creates (SPACE_TYPES in
// packages/shared); `GET /space-types` is the live list.
var SpaceTypes = []string{"DOCS", "FORUM", "BLOG", "ROADMAP", "CHANGELOG", "STATUS", "LEGAL", "FAQ", "CAREERS", "CONTACT", "EVENTS", "COURSES", "CUSTOM", "CONTENT", "ABOUT"}

type spaceModel struct {
	ID               types.String `tfsdk:"id"`
	Slug             types.String `tfsdk:"slug"`
	Name             types.String `tfsdk:"name"`
	Type             types.String `tfsdk:"type"`
	Visibility       types.String `tfsdk:"visibility"`
	Preset           types.String `tfsdk:"preset"`
	Seed             types.Bool   `tfsdk:"seed"`
	SiteID           types.String `tfsdk:"site_id"`
	Shape            types.Object `tfsdk:"shape"`
	ExternalRef      types.String `tfsdk:"external_ref"`
	Metadata         types.Map    `tfsdk:"metadata"`
	TranslationsJSON types.String `tfsdk:"translations_json"`
	SettingsJSON     types.String `tfsdk:"settings_json"`
	DestroyOnDelete  types.Bool   `tfsdk:"destroy_on_delete"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

var shapeTypes = map[string]attr.Type{
	"index": types.StringType, "order": types.StringType, "nav": types.StringType, "who_creates": types.StringType,
}

// NewSpaceResource is `forumulate_space`.
func NewSpaceResource() resource.Resource {
	return newResource(&resourceSpec[spaceModel]{
		suffix: "space",
		what:   "space",
		description: "A space: one docs site, forum, blog, changelog, status page and so on. The `type` decides its tabs, its piece names, its layouts and which other resources apply to it " +
			"(see the `forumulate_space_types` data source). Creating a space and then managing its theme, SEO, redirects and settings is `forumulate_space` plus `forumulate_space_settings`.\n\n" +
			"Destroying a space moves it to the trash (readers and the API stop seeing it; the platform can restore it). Set `destroy_on_delete = true` to make it permanent and free the slug.",
		importHelp: "the space's id or slug",
		paramAlias: map[string]string{"settings": "settings_json", "translations": "translations_json", "siteId": "site_id", "externalRef": "external_ref"},
		attrs: map[string]schema.Attribute{
			"id":   computedID("The space's id (a UUID)."),
			"slug": reqStr("The space's address segment (2 to 48 characters, lowercase letters, digits and hyphens). Cannot change; changing it replaces the space.", replStr, slug(2, 48)...),
			"name": reqStr("The space's name, 1 to 120 characters.", nil, length(1, 120)),
			"type": reqStr("The space type. One of "+joinCodes(SpaceTypes)+". Cannot change; changing it replaces the space. `GET /space-types` describes each.", replStr, oneOf(SpaceTypes...)),
			"visibility": optComputedStr("`PUBLIC` or `PRIVATE`. Defaults to `PRIVATE`. Some types are always private (the API says so on apply).",
				oneOf("PUBLIC", "PRIVATE")),
			"preset": schema.StringAttribute{
				Optional: true, Validators: []validator.String{length(1, 40)},
				PlanModifiers:       []planmodifier.String{createOnly("Change a space's layout later with the `preset` attribute of `forumulate_space_settings`.")},
				MarkdownDescription: "The named layout the reader wears, validated against the type (`GET /space-types/{type}/presets`). Create-time only: change it later with `forumulate_space_settings`.",
			},
			"seed": schema.BoolAttribute{
				Optional: true, PlanModifiers: []planmodifier.Bool{createOnlyB("")},
				MarkdownDescription: "Whether to create the starter structure this type ships with. Left unset a token-created space gets none. Create-time only.",
			},
			"site_id": schema.StringAttribute{
				Optional: true, Validators: []validator.String{uuid()},
				PlanModifiers:       []planmodifier.String{createOnly("Move a space between sites with `space_ids` on `forumulate_site`.")},
				MarkdownDescription: "A site (by id) the new public space joins. Create-time only; membership afterwards is `forumulate_site.space_ids`.",
			},
			"shape": schema.SingleNestedAttribute{
				Optional:            true,
				PlanModifiers:       []planmodifier.Object{createOnlyObj("")},
				MarkdownDescription: "How a `CUSTOM` space works: its index, order, navigation and who may create pieces. Create-time only.",
				Attributes: map[string]schema.Attribute{
					"index":       schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("landing", "list", "board", "single")}, MarkdownDescription: "`landing`, `list`, `board` or `single`."},
					"order":       schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("manual", "chronological", "activity", "votes", "status")}, MarkdownDescription: "`manual`, `chronological`, `activity`, `votes` or `status`."},
					"nav":         schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("tree", "categories", "tags", "sections", "none")}, MarkdownDescription: "`tree`, `categories`, `tags`, `sections` or `none`."},
					"who_creates": schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("staff", "members", "anyone")}, MarkdownDescription: "`staff`, `members` or `anyone`."},
				},
			},
			"external_ref": schema.StringAttribute{
				Optional: true, Validators: externalRef(), PlanModifiers: replStr,
				MarkdownDescription: "Your own reference for this space (printable ASCII, up to 200 characters), fixed at creation: repeating a create with the same reference answers the existing space.",
			},
			"metadata": schema.MapAttribute{
				Optional: true, ElementType: types.StringType, Validators: []validator.Map{metadataMap()},
				MarkdownDescription: "Tags an orchestrator keeps on the space: keys of letters, digits and `_.:-` (up to 64 characters), values up to 500 characters. Replaced whole on change. The API does not return them on a read, so drift is not detected.",
			},
			"translations_json": jsonAttr("The space's name and description in other languages: `{ \"es\": { \"name\": \"Documentación\", \"seoTitle\": \"...\" } }` per language code. Replaced whole on change."),
			"settings_json":     jsonAttr("Space settings, merged into the stored ones at the top level: each key replaces that key whole and a key you remove from the configuration is cleared. A setting that has its own door (`theme`, `seo`, `redirects`, a layout) belongs to `forumulate_space_settings` and is refused here by name."),
			"destroy_on_delete": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "On destroy, permanently delete the space after trashing it (all its content, files and domains), which frees the slug for reuse. Defaults to `false`: the space stays in the trash, restorable.",
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "When the space was created."},
		},
		create: func(ctx context.Context, c *client.Client, m *spaceModel, d *diag.Diagnostics) error {
			body := Object{"name": m.Name.ValueString(), "slug": m.Slug.ValueString(), "type": m.Type.ValueString()}
			setStr(body, "visibility", m.Visibility, false)
			setStr(body, "preset", m.Preset, false)
			setBool(body, "seed", m.Seed, false)
			setStr(body, "siteId", m.SiteID, false)
			setStr(body, "externalRef", m.ExternalRef, false)
			setStrMap(body, "metadata", m.Metadata, false)
			if known(m.Shape) {
				sh := Object{}
				attrs := m.Shape.Attributes()
				for k, wire := range map[string]string{"index": "index", "order": "order", "nav": "nav", "who_creates": "whoCreates"} {
					if v, ok := attrs[k].(types.String); ok && known(v) {
						sh[wire] = v.ValueString()
					}
				}
				body["shape"] = sh
			}
			out, err := c.DoObject(ctx, http.MethodPost, c.OrgPath("spaces"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			sp := client.Unwrap(out, "space")
			m.ID = getS(sp, "id")
			// The create body takes neither settings nor translations: one PATCH.
			patch := Object{}
			setJSON(patch, "translations", m.TranslationsJSON, false)
			mergeBodyTop(patch, "settings", types.StringNull(), m.SettingsJSON)
			if len(patch) > 0 {
				out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(m.ID.ValueString()), patch)
				if err != nil {
					return err
				}
				warnList(d, out)
				sp = client.Unwrap(out, "space")
			}
			applySpace(m, sp)
			return nil
		},
		created: func(m *spaceModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *spaceModel, d *diag.Diagnostics) error {
			ref := m.ID.ValueString()
			if ref == "" {
				ref = m.Slug.ValueString()
			}
			out, err := c.DoObject(ctx, http.MethodGet, c.SpacePath(ref), nil)
			if err != nil {
				return err
			}
			applySpace(m, client.Unwrap(out, "space"))
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *spaceModel, d *diag.Diagnostics) error {
			patch := Object{"name": plan.Name.ValueString()}
			setStr(patch, "visibility", plan.Visibility, false)
			if !plan.Metadata.Equal(prior.Metadata) {
				setStrMap(patch, "metadata", plan.Metadata, true)
			}
			if !plan.TranslationsJSON.Equal(prior.TranslationsJSON) {
				setJSON(patch, "translations", plan.TranslationsJSON, true)
			}
			if !plan.SettingsJSON.Equal(prior.SettingsJSON) {
				mergeBodyTop(patch, "settings", prior.SettingsJSON, plan.SettingsJSON)
			}
			plan.ID = prior.ID
			out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(prior.ID.ValueString()), patch)
			if err != nil {
				return err
			}
			warnList(d, out)
			applySpace(plan, client.Unwrap(out, "space"))
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *spaceModel, d *diag.Diagnostics) error {
			if err := c.Do(ctx, http.MethodDelete, c.SpacePath(m.ID.ValueString()), nil, nil); err != nil {
				return err
			}
			if m.DestroyOnDelete.ValueBool() {
				q := url.Values{"confirm": {m.Slug.ValueString()}}
				return c.Do(ctx, http.MethodDelete, c.SpacePath(m.ID.ValueString(), "destroy")+"?"+q.Encode(), nil, nil)
			}
			return nil
		},
		importID: func(id string) (map[string]string, error) { return map[string]string{"id": id}, nil },
	})
}

// applySpace copies what the API holds into the model. Settings and
// translations are read from the stored settings (`settings.translations`) and
// compared only on the keys the configuration declares.
func applySpace(m *spaceModel, sp Object) {
	if v := getS(sp, "id"); known(v) {
		m.ID = v
	}
	m.Slug = getS(sp, "slug")
	m.Name = getS(sp, "name")
	m.Type = getS(sp, "type")
	m.Visibility = getS(sp, "visibility")
	m.CreatedAt = getS(sp, "createdAt")
	settings, _ := sp["settings"].(map[string]any)
	m.SettingsJSON = jsonFromAPI(m.SettingsJSON, anyOrNil(settings))
	var tr any
	if settings != nil {
		tr = settings["translations"]
	}
	m.TranslationsJSON = jsonFromAPI(m.TranslationsJSON, tr)
	if m.Shape.IsUnknown() {
		m.Shape = types.ObjectNull(shapeTypes)
	}
	if m.Metadata.IsUnknown() {
		m.Metadata = types.MapNull(types.StringType)
	}
}

// anyOrNil is a nil interface for a nil map, so "absent" compares as absent.
func anyOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

func joinCodes(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += "`" + s + "`"
	}
	return out
}
