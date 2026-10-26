package provider

import (
	"context"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &statusMetricResource{}
	_ resource.ResourceWithImportState = &statusMetricResource{}
)

// NewStatusMetricResource is `forumulate_status_metric`: a custom metric's
// settings. Its points are pushed by your systems, not by Terraform.
func NewStatusMetricResource() resource.Resource { return &statusMetricResource{} }

type statusMetricResource struct{ c *client.Client }

type metricModel struct {
	ID          types.String  `tfsdk:"id"`
	SpaceID     types.String  `tfsdk:"space_id"`
	Key         types.String  `tfsdk:"key"`
	Name        types.String  `tfsdk:"name"`
	Unit        types.String  `tfsdk:"unit"`
	Description types.String  `tfsdk:"description"`
	Decimals    types.Int64   `tfsdk:"decimals"`
	Aggregate   types.String  `tfsdk:"aggregate"`
	Visible     types.Bool    `tfsdk:"visible"`
	SortKey     types.Int64   `tfsdk:"sort_key"`
	AxisMin     types.Float64 `tfsdk:"axis_min"`
	AxisMax     types.Float64 `tfsdk:"axis_max"`
	Threshold   types.Float64 `tfsdk:"threshold"`
}

func (r *statusMetricResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_metric"
}

func (r *statusMetricResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keepInt := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A custom metric on a status page — what it is called, its unit, how a chart bucket folds its points, whether the page draws it, its axis and threshold line.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"space_id":    schema.StringAttribute{Required: true, PlanModifiers: replace},
			"key":         schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "The key points are pushed to."},
			"name":        schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep},
			"unit":        schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, MarkdownDescription: "Printed after each value: \"ms\", \"%\"."},
			"description": schema.StringAttribute{Optional: true},
			"decimals":    schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepInt},
			"aggregate":   schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keep, MarkdownDescription: "How a chart bucket folds its points: `avg`, `max`, `min`, `sum` or `last`."},
			"visible": schema.BoolAttribute{
				Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"sort_key":  schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepInt},
			"axis_min":  schema.Float64Attribute{Optional: true},
			"axis_max":  schema.Float64Attribute{Optional: true},
			"threshold": schema.Float64Attribute{Optional: true, MarkdownDescription: "A dashed line across the chart at this value."},
		},
	}
}

func (r *statusMetricResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.c = c
	}
}

func metricBody(m metricModel, update bool) map[string]any {
	b := map[string]any{}
	for key, v := range map[string]types.String{"name": m.Name, "unit": m.Unit, "aggregate": m.Aggregate} {
		if known(v) {
			b[key] = v.ValueString()
		}
	}
	setNullable(b, "description", m.Description, update)
	for key, v := range map[string]types.Int64{"decimals": m.Decimals, "sortKey": m.SortKey} {
		if known(v) {
			b[key] = v.ValueInt64()
		}
	}
	if known(m.Visible) {
		b["visible"] = m.Visible.ValueBool()
	}
	setNullableFloat(b, "axisMin", m.AxisMin, update)
	setNullableFloat(b, "axisMax", m.AxisMax, update)
	setNullableFloat(b, "threshold", m.Threshold, update)
	return b
}

func fromMetric(m *metricModel, x *client.Metric) {
	m.ID = types.StringValue(m.SpaceID.ValueString() + "/" + x.Key)
	m.Key = types.StringValue(x.Key)
	m.Name = types.StringValue(x.Name)
	m.Unit = types.StringValue(x.Unit)
	m.Description = strOrNull(x.Description)
	m.Decimals = types.Int64Value(x.Decimals)
	m.Aggregate = types.StringValue(x.Aggregate)
	m.Visible = types.BoolValue(x.Visible)
	m.SortKey = types.Int64Value(x.SortKey)
	m.AxisMin = floatOrNull(x.AxisMin)
	m.AxisMax = floatOrNull(x.AxisMax)
	m.Threshold = floatOrNull(x.Threshold)
}

func (r *statusMetricResource) put(ctx context.Context, m *metricModel, update bool) error {
	x, err := r.c.PutMetric(ctx, m.SpaceID.ValueString(), m.Key.ValueString(), metricBody(*m, update))
	if err != nil {
		return err
	}
	fromMetric(m, x)
	return nil
}

func (r *statusMetricResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan metricModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.put(ctx, &plan, false); err != nil {
		apiDiag(&resp.Diagnostics, "creating a status metric", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusMetricResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state metricModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	x, err := r.c.GetMetric(ctx, state.SpaceID.ValueString(), state.Key.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiag(&resp.Diagnostics, "reading a status metric", err)
		return
	}
	fromMetric(&state, x)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusMetricResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan metricModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.put(ctx, &plan, true); err != nil {
		apiDiag(&resp.Diagnostics, "updating a status metric", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusMetricResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state metricModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	if err := r.c.DeleteMetric(ctx, state.SpaceID.ValueString(), state.Key.ValueString()); err != nil && !client.IsNotFound(err) {
		apiDiag(&resp.Diagnostics, "deleting a status metric", err)
	}
}

// ImportState takes `<space_id>/<key>`.
func (r *statusMetricResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	space, key, ok := strings.Cut(req.ID, "/")
	if !ok || space == "" || key == "" {
		resp.Diagnostics.AddError("Import id", "Use <space_id>/<metric key>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), space)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), key)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
