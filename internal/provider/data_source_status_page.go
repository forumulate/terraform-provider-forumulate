package provider

import (
	"context"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &statusPageDataSource{}

// NewStatusPageDataSource is `forumulate_status_page`: what a page says right
// now, read from its public widget.json — a deploy pipeline can refuse to roll
// out while the page reports an outage.
func NewStatusPageDataSource() datasource.DataSource { return &statusPageDataSource{} }

type statusPageDataSource struct{ c *client.Client }

type statusPageModel struct {
	SpaceID     types.String `tfsdk:"space_id"`
	Name        types.String `tfsdk:"name"`
	URL         types.String `tfsdk:"url"`
	Overall     types.String `tfsdk:"overall"`
	Indicator   types.String `tfsdk:"indicator"`
	Description types.String `tfsdk:"description"`
	ActiveCount types.Int64  `tfsdk:"active_count"`
}

func (d *statusPageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (d *statusPageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The current state of a public status page.",
		Attributes: map[string]schema.Attribute{
			"space_id":     schema.StringAttribute{Required: true},
			"name":         schema.StringAttribute{Computed: true},
			"url":          schema.StringAttribute{Computed: true},
			"overall":      schema.StringAttribute{Computed: true, MarkdownDescription: "`up`, `degraded`, `maintenance`, `down` or `unknown`."},
			"indicator":    schema.StringAttribute{Computed: true, MarkdownDescription: "Statuspage's word: `none`, `minor`, `major`, `critical` or `maintenance`."},
			"description":  schema.StringAttribute{Computed: true, MarkdownDescription: "The page's headline, in the owner's words."},
			"active_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Open incidents and maintenance under way."},
		},
	}
}

func (d *statusPageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.c = c
	}
}

func (d *statusPageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m statusPageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.c == nil {
		resp.Diagnostics.AddError("Provider not configured", "The forumulate provider has no configuration.")
		return
	}
	st, err := d.c.GetPageState(ctx, m.SpaceID.ValueString())
	if err != nil {
		apiDiag(&resp.Diagnostics, "reading a status page", err)
		return
	}
	m.Name = types.StringValue(st.Page.Name)
	m.URL = types.StringValue(st.Page.URL)
	m.Overall = types.StringValue(st.Status.Overall)
	m.Indicator = types.StringValue(st.Status.Indicator)
	m.Description = types.StringValue(st.Status.Description)
	m.ActiveCount = types.Int64Value(int64(len(st.Active)))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
