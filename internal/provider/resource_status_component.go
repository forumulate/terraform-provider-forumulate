package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &statusComponentResource{}
	_ resource.ResourceWithImportState = &statusComponentResource{}
)

// NewStatusComponentResource is `forumulate_status_component`: one component
// (monitor) of a status page.
func NewStatusComponentResource() resource.Resource { return &statusComponentResource{} }

type statusComponentResource struct{ c *client.Client }

type componentModel struct {
	ID               types.String `tfsdk:"id"`
	SpaceID          types.String `tfsdk:"space_id"`
	Name             types.String `tfsdk:"name"`
	Kind             types.String `tfsdk:"kind"`
	URL              types.String `tfsdk:"url"`
	Method           types.String `tfsdk:"method"`
	Group            types.String `tfsdk:"group"`
	Description      types.String `tfsdk:"description"`
	Slug             types.String `tfsdk:"slug"`
	LinkURL          types.String `tfsdk:"link_url"`
	Display          types.String `tfsdk:"display"`
	IntervalSeconds  types.Int64  `tfsdk:"interval_seconds"`
	Visible          types.Bool   `tfsdk:"visible"`
	AlertsEnabled    types.Bool   `tfsdk:"alerts_enabled"`
	FailureThreshold types.Int64  `tfsdk:"failure_threshold"`
	Keyword          types.String `tfsdk:"keyword"`
	KeywordAbsent    types.Bool   `tfsdk:"keyword_absent"`
	ExpectStatus     types.Int64  `tfsdk:"expect_status"`
	CertExpiryDays   types.Int64  `tfsdk:"cert_expiry_days"`
	Region           types.String `tfsdk:"region"`
	Regions          types.List   `tfsdk:"regions"`
	ConfirmRegions   types.Int64  `tfsdk:"confirm_regions"`
	BarDays          types.Int64  `tfsdk:"bar_days"`
	ShowUptime       types.Bool   `tfsdk:"show_uptime"`
	Paused           types.Bool   `tfsdk:"paused"`
	ProbeJSON        types.String `tfsdk:"probe_json"`
	PingURL          types.String `tfsdk:"ping_url"`
}

func (r *statusComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_component"
}

func (r *statusComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	optStr := func(d string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, MarkdownDescription: d}
	}
	optComputedStr := func(d string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: d, PlanModifiers: keep}
	}
	optInt := func(d string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, MarkdownDescription: d}
	}
	optComputedInt := func(d string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: d,
			PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}}
	}
	optComputedBool := func(d string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: d,
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A component of a status page: an HTTP, heartbeat, protocol (TCP, UDP, ping, DNS, TLS, SMTP, IMAP, POP3, domain), browser, manual, rolled-up or vendor-mirrored check.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"space_id": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "The status page (space) id."},
			"name":     schema.StringAttribute{Required: true},
			"kind": schema.StringAttribute{
				Optional: true, Computed: true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "`http` (default), `heartbeat`, `manual`, `space`, `statuspage`, `instatus`, `betterstack`, `feed`, `tcp`, `udp`, `icmp`, `dns`, `tls`, `smtp`, `imap`, `pop3`, `domain` or `browser`. Fixed at creation.",
			},
			"url":               optStr("The address to poll (`http`) or the vendor page (`statuspage`, `instatus`, `betterstack`, `feed`)."),
			"method":            optComputedStr("`GET` (default), `HEAD` or `POST`."),
			"group":             optStr("The group it is listed under."),
			"description":       optStr("A sentence shown beside its name."),
			"slug":              optComputedStr("Its stable address within the page; minted from the name when unset."),
			"link_url":          optStr("Where its own docs or product live."),
			"display":           optComputedStr("`bars` (default), `status` or `hidden-unless-degraded`."),
			"interval_seconds":  optComputedInt("Seconds between checks, or the longest wait for a heartbeat's ping."),
			"visible":           optComputedBool("Listed on the public page. Default true."),
			"alerts_enabled":    optComputedBool("Alerts fire when it goes down. Default true."),
			"failure_threshold": optComputedInt("Failed checks in a row before it reads down (1–10)."),
			"keyword":           optStr("A term the body must contain (or must not, with `keyword_absent`)."),
			"keyword_absent":    optComputedBool("Fail while `keyword` is in the body."),
			"expect_status":     optInt("The exact HTTP status that means up; unset keeps the under-400 rule."),
			"cert_expiry_days":  optInt("Days a TLS certificate must have left (https)."),
			"region":            optStr("Where it runs (\"EU\"), for grouping by region."),
			"regions": schema.ListAttribute{
				ElementType: types.StringType, Optional: true, Computed: true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "Probe regions it is checked from (see the API's `GET …/status/regions`). Empty: the primary checker.",
			},
			"confirm_regions": optComputedInt("How many regions must see it failing before it reads down."),
			"bar_days":        optInt("Its own day strip length on the board: 30, 60 or 90. Unset follows the theme."),
			"show_uptime":     optComputedBool("Print its uptime figure beside the strip. Default true."),
			"paused":          optComputedBool("Kept on the page with its history, not probed."),
			"probe_json": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A protocol or browser check's probe as JSON (`jsonencode({ host = \"db.example.com\", port = 5432 })`): `host`, `port`, `record`, `resolver`, `expect`, `payload`, `days`, `url`, `selector`.",
			},
			"ping_url": schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "Heartbeat only: where the job pings."},
		},
	}
}

func (r *statusComponentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.c = c
	}
}

// componentBody is the API body for a plan. On update, every clearable field
// the configuration no longer sets is sent as null so the server clears it.
func componentBody(ctx context.Context, m componentModel, update bool) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	b := map[string]any{"name": m.Name.ValueString()}
	if !update && known(m.Kind) {
		b["kind"] = m.Kind.ValueString()
	}
	if known(m.URL) {
		b["url"] = m.URL.ValueString()
	}
	for key, v := range map[string]types.String{"method": m.Method, "slug": m.Slug, "display": m.Display} {
		if known(v) {
			b[key] = v.ValueString()
		}
	}
	setNullable(b, "group", m.Group, update)
	setNullable(b, "description", m.Description, update)
	setNullable(b, "linkUrl", m.LinkURL, update)
	setNullable(b, "keyword", m.Keyword, update)
	setNullable(b, "region", m.Region, update)
	setNullableInt(b, "expectStatus", m.ExpectStatus, update)
	setNullableInt(b, "certExpiryDays", m.CertExpiryDays, update)
	setNullableInt(b, "barDays", m.BarDays, update)
	for key, v := range map[string]types.Int64{
		"intervalSeconds": m.IntervalSeconds, "failureThreshold": m.FailureThreshold, "confirmRegions": m.ConfirmRegions,
	} {
		if known(v) {
			b[key] = v.ValueInt64()
		}
	}
	for key, v := range map[string]types.Bool{
		"visible": m.Visible, "alertsEnabled": m.AlertsEnabled, "keywordAbsent": m.KeywordAbsent,
		"showUptime": m.ShowUptime, "paused": m.Paused,
	} {
		if known(v) {
			b[key] = v.ValueBool()
		}
	}
	if known(m.Regions) {
		var regions []string
		diags.Append(m.Regions.ElementsAs(ctx, &regions, false)...)
		b["regions"] = regions
	}
	if known(m.ProbeJSON) {
		var probe map[string]any
		if err := json.Unmarshal([]byte(m.ProbeJSON.ValueString()), &probe); err != nil {
			diags.AddAttributeError(path.Root("probe_json"), "probe_json is not a JSON object", err.Error())
		} else {
			b["probe"] = probe
		}
	}
	return b, diags
}

// fromMonitor writes what the API holds into the model, keeping the prior
// probe_json spelling when it means the same thing.
func fromMonitor(m *componentModel, mon *client.Monitor) {
	m.ID = types.StringValue(mon.ID)
	m.Name = types.StringValue(mon.Name)
	m.Kind = types.StringValue(mon.Kind)
	if mon.URL != "" || known(m.URL) {
		m.URL = types.StringValue(mon.URL)
	}
	m.Method = types.StringValue(mon.Method)
	m.Group = strOrNull(mon.Group)
	m.Description = strOrNull(mon.Description)
	m.Slug = strOrNull(mon.Slug)
	m.LinkURL = strOrNull(mon.LinkURL)
	m.Display = types.StringValue(mon.Display)
	m.IntervalSeconds = types.Int64Value(mon.IntervalSeconds)
	m.Visible = types.BoolValue(mon.Visible)
	m.AlertsEnabled = types.BoolValue(mon.AlertsEnabled)
	m.FailureThreshold = types.Int64Value(mon.FailureThreshold)
	m.Keyword = strOrNull(mon.Keyword)
	m.KeywordAbsent = types.BoolValue(mon.KeywordAbsent)
	m.ExpectStatus = intOrNull(mon.ExpectStatus)
	m.CertExpiryDays = intOrNull(mon.CertExpiryDays)
	m.Region = strOrNull(mon.Region)
	regions := make([]attr.Value, 0, len(mon.Regions))
	for _, r := range mon.Regions {
		regions = append(regions, types.StringValue(r))
	}
	m.Regions = types.ListValueMust(types.StringType, regions)
	m.ConfirmRegions = types.Int64Value(mon.ConfirmRegions)
	m.BarDays = intOrNull(mon.BarDays)
	m.ShowUptime = types.BoolValue(mon.ShowUptime)
	m.Paused = types.BoolValue(mon.Paused)
	m.ProbeJSON = probeState(m.ProbeJSON, mon.Probe)
	if mon.PingURL != "" {
		m.PingURL = types.StringValue(mon.PingURL)
	} else {
		m.PingURL = types.StringNull()
	}
}

func (r *statusComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if needsToken(r.c, &resp.Diagnostics) {
		return
	}
	var plan componentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	body, d := componentBody(ctx, plan, false)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	mon, err := r.c.CreateMonitor(ctx, plan.SpaceID.ValueString(), body)
	if err != nil {
		apiDiag(&resp.Diagnostics, "creating a status component", err)
		return
	}
	fromMonitor(&plan, mon)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state componentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	mon, err := r.c.GetMonitor(ctx, state.SpaceID.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiag(&resp.Diagnostics, "reading a status component", err)
		return
	}
	fromMonitor(&state, mon)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if needsToken(r.c, &resp.Diagnostics) {
		return
	}
	var plan, state componentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	body, d := componentBody(ctx, plan, true)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	mon, err := r.c.UpdateMonitor(ctx, state.SpaceID.ValueString(), state.ID.ValueString(), body)
	if err != nil {
		apiDiag(&resp.Diagnostics, "updating a status component", err)
		return
	}
	plan.ID = state.ID
	fromMonitor(&plan, mon)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state componentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || needsToken(r.c, &resp.Diagnostics) {
		return
	}
	err := r.c.DeleteMonitor(ctx, state.SpaceID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		apiDiag(&resp.Diagnostics, "deleting a status component", err)
	}
}

// ImportState takes `<space_id>/<component_id>`.
func (r *statusComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	space, id, ok := strings.Cut(req.ID, "/")
	if !ok || space == "" || id == "" {
		resp.Diagnostics.AddError("Import id", "Use <space_id>/<component_id>.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), space)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
