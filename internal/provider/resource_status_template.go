package provider

import (
	"context"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &statusTemplateResource{}
	_ resource.ResourceWithImportState = &statusTemplateResource{}
)

// NewStatusIncidentTemplateResource is `forumulate_status_incident_template`.
func NewStatusIncidentTemplateResource() resource.Resource { return &statusTemplateResource{} }

type statusTemplateResource struct{ c *client.Client }

type templateModel struct {
	ID         types.String `tfsdk:"id"`
	SpaceID    types.String `tfsdk:"space_id"`
	Name       types.String `tfsdk:"name"`
	Title      types.String `tfsdk:"title"`
	Severity   types.String `tfsdk:"severity"`
	State      types.String `tfsdk:"state"`
	Markdown   types.String `tfsdk:"markdown"`
	Identified types.String `tfsdk:"identified_text"`
	Monitoring types.String `tfsdk:"monitoring_text"`
	Resolved   types.String `tfsdk:"resolved_text"`
}

func (r *statusTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_incident_template"
}

func (r *statusTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An incident template: what an incident opens with, and the words offered for each later step. `{{name}}` variables are filled when it is used.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"space_id": schema.StringAttribute{
				Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":     schema.StringAttribute{Required: true, MarkdownDescription: "What it is called in the picker."},
			"title":    schema.StringAttribute{Required: true, MarkdownDescription: "The title of the incident it opens."},
			"severity": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, MarkdownDescription: "`minor`, `major`, `critical`, `maintenance` or `notice`."},
			"state":    schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, MarkdownDescription: "The state it opens in. Default `investigating`."},
			"markdown": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The write-up, in Markdown. Stored as a block document, so it is sent on change and not read back.",
			},
			"identified_text": schema.StringAttribute{Optional: true, MarkdownDescription: "The words offered when an update moves it to identified."},
			"monitoring_text": schema.StringAttribute{Optional: true, MarkdownDescription: "…to monitoring."},
			"resolved_text":   schema.StringAttribute{Optional: true, MarkdownDescription: "…to resolved."},
		},
	}
}

func (r *statusTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.c = c
	}
}

func templateBody(m templateModel) map[string]any {
	b := map[string]any{"name": m.Name.ValueString(), "title": m.Title.ValueString()}
	if known(m.Severity) {
		b["severity"] = m.Severity.ValueString()
	}
	if known(m.State) {
		b["state"] = m.State.ValueString()
	}
	if known(m.Markdown) {
		b["markdown"] = m.Markdown.ValueString()
	}
	updates := map[string]string{}
	for key, v := range map[string]types.String{"identified": m.Identified, "monitoring": m.Monitoring, "resolved": m.Resolved} {
		if known(v) && v.ValueString() != "" {
			updates[key] = v.ValueString()
		}
	}
	b["updates"] = updates
	return b
}

func fromTemplate(m *templateModel, t *client.Template) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.Title = types.StringValue(t.Title)
	m.Severity = types.StringValue(t.Severity)
	m.State = types.StringValue(t.State)
	text := func(k string) types.String {
		if v, ok := t.Updates[k]; ok && v != "" {
			return types.StringValue(v)
		}
		return types.StringNull()
	}
	m.Identified = text("identified")
	m.Monitoring = text("monitoring")
	m.Resolved = text("resolved")
}

func (r *statusTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	t, err := r.c.CreateTemplate(ctx, plan.SpaceID.ValueString(), templateBody(plan))
	if err != nil {
		apiDiag(&resp.Diagnostics, "creating an incident template", err)
		return
	}
	fromTemplate(&plan, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	t, err := r.c.GetTemplate(ctx, state.SpaceID.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiag(&resp.Diagnostics, "reading an incident template", err)
		return
	}
	fromTemplate(&state, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state templateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	body := templateBody(plan)
	// The write-up is not read back; only send it when it changed, and clear
	// it when the configuration dropped it.
	if plan.Markdown.Equal(state.Markdown) {
		delete(body, "markdown")
	} else if plan.Markdown.IsNull() {
		body["body"] = nil
	}
	t, err := r.c.UpdateTemplate(ctx, state.SpaceID.ValueString(), state.ID.ValueString(), body)
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating an incident template", err)
		return
	}
	fromTemplate(&plan, t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state templateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.c.DeleteTemplate(ctx, state.SpaceID.ValueString(), state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiDiag(&resp.Diagnostics, "deleting an incident template", err)
	}
}

// ImportState takes `<space_id>/<template_id>`.
func (r *statusTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	space, id, ok := strings.Cut(req.ID, "/")
	if !ok || space == "" || id == "" {
		resp.Diagnostics.AddError("Import id", "Use <space_id>/<template_id>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), space)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
