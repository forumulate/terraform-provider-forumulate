package provider

import (
	"context"
	"net/http"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// dataSource is the plumbing the read-only data sources share: the schema, a
// token check and an error mapping.
type dataSource[M any] struct {
	suffix string
	what   string
	schema schema.Schema
	read   func(ctx context.Context, c *client.Client, m *M, d *diag.Diagnostics) error
	c      *client.Client
}

func (d *dataSource[M]) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.suffix
}

func (d *dataSource[M]) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = d.schema
}

func (d *dataSource[M]) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.c = c
	}
}

func (d *dataSource[M]) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m M
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || needsToken(d.c, &resp.Diagnostics) {
		return
	}
	if err := d.read(ctx, d.c, &m, &resp.Diagnostics); err != nil {
		apiDiag(&resp.Diagnostics, "reading "+d.what, err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// ---- forumulate_space ----

type spaceDataModel struct {
	ID           types.String `tfsdk:"id"`
	Slug         types.String `tfsdk:"slug"`
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	Visibility   types.String `tfsdk:"visibility"`
	SettingsJSON types.String `tfsdk:"settings_json"`
	ItemCount    types.Int64  `tfsdk:"item_count"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

// NewSpaceDataSource is the `forumulate_space` data source.
func NewSpaceDataSource() datasource.DataSource {
	return &dataSource[spaceDataModel]{
		suffix: "space", what: "space",
		schema: schema.Schema{
			MarkdownDescription: "Looks up a space by its slug (or id): the id every other resource wants, its type and visibility, and its stored settings.",
			Attributes: map[string]schema.Attribute{
				"slug":          schema.StringAttribute{Required: true, MarkdownDescription: "The space's slug (or its id)."},
				"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "The space's id (a UUID)."},
				"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "The space's name."},
				"type":          schema.StringAttribute{Computed: true, MarkdownDescription: "The space type, such as `DOCS` or `FORUM`."},
				"visibility":    schema.StringAttribute{Computed: true, MarkdownDescription: "`PUBLIC` or `PRIVATE`."},
				"settings_json": schema.StringAttribute{Computed: true, MarkdownDescription: "The space's stored settings, as JSON. Decode with `jsondecode()`."},
				"item_count":    schema.Int64Attribute{Computed: true, MarkdownDescription: "How many pieces (pages, posts, topics) the space holds."},
				"created_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "When the space was created."},
			},
		},
		read: func(ctx context.Context, c *client.Client, m *spaceDataModel, d *diag.Diagnostics) error {
			out, err := c.DoObject(ctx, http.MethodGet, c.SpacePath(m.Slug.ValueString()), nil)
			if err != nil {
				if client.IsNotFound(err) {
					d.AddAttributeError(attrPath("slug"), "No such space", "The organization has no space \""+m.Slug.ValueString()+"\".")
					return nil
				}
				return err
			}
			sp := client.Unwrap(out, "space")
			m.ID = getS(sp, "id")
			m.Slug = getS(sp, "slug")
			m.Name = getS(sp, "name")
			m.Type = getS(sp, "type")
			m.Visibility = getS(sp, "visibility")
			m.ItemCount = getI(sp, "itemCount")
			m.CreatedAt = getS(sp, "createdAt")
			m.SettingsJSON = types.StringValue(canonical(sp["settings"]))
			return nil
		},
	}
}

// ---- forumulate_org ----

type orgDataModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Slug               types.String `tfsdk:"slug"`
	ContentLanguage    types.String `tfsdk:"content_language"`
	InboundEmailDomain types.String `tfsdk:"inbound_email_domain"`
	TrashRetentionDays types.Int64  `tfsdk:"trash_retention_days"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

// NewOrgDataSource is the `forumulate_org` data source.
func NewOrgDataSource() datasource.DataSource {
	return &dataSource[orgDataModel]{
		suffix: "org", what: "organization",
		schema: schema.Schema{
			MarkdownDescription: "The organization the provider is configured for (`org_id` / `FORUMULATE_ORG_ID`): its id, slug, name and base settings.",
			Attributes: map[string]schema.Attribute{
				"id":                   schema.StringAttribute{Computed: true, MarkdownDescription: "The organization's id (a UUID)."},
				"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "The organization's name."},
				"slug":                 schema.StringAttribute{Computed: true, MarkdownDescription: "The organization's slug."},
				"content_language":     schema.StringAttribute{Computed: true, MarkdownDescription: "The organization's content language, when set."},
				"inbound_email_domain": schema.StringAttribute{Computed: true, MarkdownDescription: "The domain inbound email is received on, when set."},
				"trash_retention_days": schema.Int64Attribute{Computed: true, MarkdownDescription: "How long trashed content is kept, when set."},
				"created_at":           schema.StringAttribute{Computed: true, MarkdownDescription: "When the organization was created."},
			},
		},
		read: func(ctx context.Context, c *client.Client, m *orgDataModel, d *diag.Diagnostics) error {
			out, err := c.DoObject(ctx, http.MethodGet, c.OrgPath(), nil)
			if err != nil {
				return err
			}
			o := client.Unwrap(out, "organization")
			m.ID = getS(o, "id")
			m.Name = getS(o, "name")
			m.Slug = getS(o, "slug")
			m.ContentLanguage = getS(o, "contentLanguage")
			m.InboundEmailDomain = getS(o, "inboundEmailDomain")
			m.TrashRetentionDays = getI(o, "trashRetentionDays")
			m.CreatedAt = getS(o, "createdAt")
			return nil
		},
	}
}

// ---- forumulate_space_types ----

type spaceTypesModel struct {
	ID    types.String `tfsdk:"id"`
	Names types.List   `tfsdk:"names"`
	Types types.List   `tfsdk:"types"`
}

var spaceTypeAttrTypes = map[string]attr.Type{
	"type": types.StringType, "name": types.StringType, "description": types.StringType, "purpose": types.StringType,
	"backing": types.StringType, "forced_visibility": types.StringType, "default_slug": types.StringType,
	"piece_names":        types.ListType{ElemType: types.StringType},
	"container_names":    types.ListType{ElemType: types.StringType},
	"tabs":               types.ListType{ElemType: types.StringType},
	"presets":            types.ListType{ElemType: types.StringType},
	"recommended_preset": types.StringType,
	"piece_presets":      types.ListType{ElemType: types.StringType},
}

func namesOf(raw any) []string {
	arr, _ := raw.([]any)
	out := []string{}
	for _, e := range arr {
		switch x := e.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			if n, ok := x["name"].(string); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

// NewSpaceTypesDataSource is the `forumulate_space_types` data source.
func NewSpaceTypesDataSource() datasource.DataSource {
	strList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	return &dataSource[spaceTypesModel]{
		suffix: "space_types", what: "space types",
		schema: schema.Schema{
			MarkdownDescription: "The space-type vocabulary (`GET /space-types`): for each type what a piece and a container are called, its tabs and its layout presets. " +
				"Read it instead of hard-coding which layouts a type accepts.",
			Attributes: map[string]schema.Attribute{
				"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Always `space-types`."},
				"names": strList("The type codes, such as `DOCS`, `FORUM`, `BLOG`."),
				"types": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "Every space type.",
					NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
						"type":               schema.StringAttribute{Computed: true, MarkdownDescription: "The type code."},
						"name":               schema.StringAttribute{Computed: true, MarkdownDescription: "Its display name."},
						"description":        schema.StringAttribute{Computed: true, MarkdownDescription: "What it is."},
						"purpose":            schema.StringAttribute{Computed: true, MarkdownDescription: "What it is for."},
						"backing":            schema.StringAttribute{Computed: true, MarkdownDescription: "What a piece is stored as: `page`, `topic`, `event` or `incident`."},
						"forced_visibility":  schema.StringAttribute{Computed: true, MarkdownDescription: "`PRIVATE` for a type that is always private."},
						"default_slug":       schema.StringAttribute{Computed: true, MarkdownDescription: "The slug a space of this type gets by default."},
						"piece_names":        strList("What a piece is called (`page`, `article`, `topic`)."),
						"container_names":    strList("What a container is called (`collection`, `category`)."),
						"tabs":               strList("The console tabs the type has."),
						"presets":            strList("The layouts `preset` accepts for this type."),
						"recommended_preset": schema.StringAttribute{Computed: true, MarkdownDescription: "The layout used when none is chosen."},
						"piece_presets":      strList("The layouts `piece_preset` accepts."),
					}},
				},
			},
		},
		read: func(ctx context.Context, c *client.Client, m *spaceTypesModel, d *diag.Diagnostics) error {
			rows, err := c.List(ctx, "/space-types", "spaceTypes", nil)
			if err != nil {
				return err
			}
			names := []string{}
			vals := []attr.Value{}
			for _, r := range rows {
				code := getS(r, "type").ValueString()
				names = append(names, code)
				recommended := ""
				if arr, ok := r["presets"].([]any); ok {
					for _, p := range arr {
						if o, ok := p.(map[string]any); ok {
							if rec, _ := o["recommended"].(bool); rec {
								recommended, _ = o["name"].(string)
							}
						}
					}
				}
				vocab, _ := r["vocabulary"].(map[string]any)
				route, _ := r["route"].(map[string]any)
				obj, _ := types.ObjectValue(spaceTypeAttrTypes, map[string]attr.Value{
					"type": types.StringValue(code), "name": getS(r, "name"), "description": getS(r, "description"), "purpose": getS(r, "purpose"),
					"backing": getS(r, "backing"), "forced_visibility": getS(r, "forcedVisibility"), "default_slug": getS(route, "defaultSlug"),
					"piece_names":        listOfStrings(namesOf(vocab["piece"])),
					"container_names":    listOfStrings(namesOf(vocab["container"])),
					"tabs":               listOfStrings(namesOf(r["tabs"])),
					"presets":            listOfStrings(namesOf(r["presets"])),
					"recommended_preset": types.StringValue(recommended),
					"piece_presets":      listOfStrings(namesOf(r["piecePresets"])),
				})
				vals = append(vals, obj)
			}
			m.ID = types.StringValue("space-types")
			m.Names = listOfStrings(names)
			m.Types, _ = types.ListValue(types.ObjectType{AttrTypes: spaceTypeAttrTypes}, vals)
			return nil
		},
	}
}
