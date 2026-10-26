package provider

import (
	"context"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &statusGroupsResource{}
	_ resource.ResourceWithImportState = &statusGroupsResource{}
)

// NewStatusGroupsResource is `forumulate_status_groups`: a page's whole group
// arrangement, in order. One per page — the API stores the arrangement as one
// list, so managing groups one at a time would have two resources racing to
// rewrite the same list.
func NewStatusGroupsResource() resource.Resource { return &statusGroupsResource{} }

type statusGroupsResource struct{ c *client.Client }

type groupModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Parent      types.String `tfsdk:"parent"`
	Collapsed   types.Bool   `tfsdk:"collapsed"`
	Layout      types.String `tfsdk:"layout"`
	HideUptime  types.Bool   `tfsdk:"hide_uptime"`
}

type groupsModel struct {
	ID      types.String `tfsdk:"id"`
	SpaceID types.String `tfsdk:"space_id"`
	Groups  []groupModel `tfsdk:"group"`
}

func (r *statusGroupsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_groups"
}

func (r *statusGroupsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The component groups of a status page, in page order: what each says about itself, which sits inside which, which starts folded, its row layout and whether its rows print uptime.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"space_id": schema.StringAttribute{
				Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "The status page (space) id.",
			},
		},
		Blocks: map[string]schema.Block{
			"group": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Required: true},
						"description": schema.StringAttribute{Optional: true},
						"parent":      schema.StringAttribute{Optional: true, MarkdownDescription: "The group it sits inside (one level)."},
						"collapsed":   schema.BoolAttribute{Optional: true, MarkdownDescription: "Starts folded."},
						"layout":      schema.StringAttribute{Optional: true, MarkdownDescription: "`list`, `grid-2` or `cards`; unset follows the page."},
						"hide_uptime": schema.BoolAttribute{Optional: true, MarkdownDescription: "Its rows print no uptime figure."},
					},
				},
			},
		},
	}
}

func (r *statusGroupsResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.c = c
	}
}

func optPtr(v types.String) *string {
	if !known(v) {
		return nil
	}
	s := v.ValueString()
	return &s
}

func groupsBody(m groupsModel) []client.Group {
	out := make([]client.Group, 0, len(m.Groups))
	for _, g := range m.Groups {
		out = append(out, client.Group{
			Name:        g.Name.ValueString(),
			Description: optPtr(g.Description),
			Parent:      optPtr(g.Parent),
			Collapsed:   known(g.Collapsed) && g.Collapsed.ValueBool(),
			Layout:      optPtr(g.Layout),
			HideUptime:  known(g.HideUptime) && g.HideUptime.ValueBool(),
		})
	}
	return out
}

// fromGroups keeps a false/unset bool as configured (null stays null) so a
// configuration that never mentions `collapsed` does not drift to false.
func fromGroups(m *groupsModel, gs []client.Group) {
	prior := map[string]groupModel{}
	for _, g := range m.Groups {
		prior[g.Name.ValueString()] = g
	}
	m.Groups = make([]groupModel, 0, len(gs))
	for _, g := range gs {
		p, had := prior[g.Name]
		// The API lists groups components name but nobody arranged after the
		// arranged ones. A bare one this configuration never named is not
		// managed here, so it is not drift (on import, with nothing prior,
		// everything is kept).
		bare := g.Description == nil && g.Parent == nil && !g.Collapsed && g.Layout == nil && !g.HideUptime
		if len(prior) > 0 && !had && bare {
			continue
		}
		boolOf := func(v bool, was types.Bool) types.Bool {
			if !v && had && was.IsNull() {
				return types.BoolNull()
			}
			if !v && !had {
				return types.BoolNull()
			}
			return types.BoolValue(v)
		}
		m.Groups = append(m.Groups, groupModel{
			Name:        types.StringValue(g.Name),
			Description: emptyAsNull(g.Description),
			Parent:      emptyAsNull(g.Parent),
			Collapsed:   boolOf(g.Collapsed, p.Collapsed),
			Layout:      emptyAsNull(g.Layout),
			HideUptime:  boolOf(g.HideUptime, p.HideUptime),
		})
	}
}

func emptyAsNull(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func (r *statusGroupsResource) write(ctx context.Context, m *groupsModel) error {
	gs, err := r.c.SetGroups(ctx, m.SpaceID.ValueString(), groupsBody(*m))
	if err != nil {
		return err
	}
	m.ID = m.SpaceID
	fromGroups(m, gs)
	return nil
}

func (r *statusGroupsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		apiDiag(&resp.Diagnostics, "setting status groups", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusGroupsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	gs, err := r.c.GetGroups(ctx, state.SpaceID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiag(&resp.Diagnostics, "reading status groups", err)
		return
	}
	state.ID = state.SpaceID
	fromGroups(&state, gs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusGroupsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		apiDiag(&resp.Diagnostics, "setting status groups", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the arrangement; the components keep their group names and
// are listed in first-seen order, as on a page that never arranged groups.
func (r *statusGroupsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if _, err := r.c.SetGroups(ctx, state.SpaceID.ValueString(), []client.Group{}); err != nil && !client.IsNotFound(err) {
		apiDiag(&resp.Diagnostics, "clearing status groups", err)
	}
}

// ImportState takes the space id.
func (r *statusGroupsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
