// Package provider is the Terraform provider: its configuration, and the
// status page resources and data source built on internal/client.
package provider

import (
	"context"
	"os"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &ForumulateProvider{}

// ForumulateProvider holds only the build version; the configured client is
// made in Configure and handed to every resource and data source.
type ForumulateProvider struct {
	version string
}

type providerModel struct {
	APIURL types.String `tfsdk:"api_url"`
	Token  types.String `tfsdk:"token"`
	OrgID  types.String `tfsdk:"org_id"`
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &ForumulateProvider{version: version} }
}

func (p *ForumulateProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "forumulate"
	resp.Version = p.version
}

func (p *ForumulateProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a Forumulate organization as code: spaces and their pages, collections, forum categories, sites, domains, webhooks, settings, connectors and status pages, or the whole site at once as a manifest. Authenticates with a personal access token; every request pins `Forumulate-Version: 2027-02-01`.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				MarkdownDescription: "The API base, `https://forumulate.com/api` by default, or `FORUMULATE_API_URL`.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "A personal access token with access to the organization. Prefer `FORUMULATE_TOKEN` so it never lands in configuration.",
				Optional:            true,
				Sensitive:           true,
			},
			"org_id": schema.StringAttribute{
				MarkdownDescription: "The organization's id, or `FORUMULATE_ORG_ID`.",
				Optional:            true,
			},
		},
	}
}

// pick is the configured value, else the environment's, else the fallback —
// configuration wins so a module can pin a value the environment would change.
func pick(v types.String, env, fallback string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	if e := os.Getenv(env); e != "" {
		return e
	}
	return fallback
}

func (p *ForumulateProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token := pick(cfg.Token, "FORUMULATE_TOKEN", "")
	orgID := pick(cfg.OrgID, "FORUMULATE_ORG_ID", "")
	if orgID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("org_id"), "Missing organization id",
			"Set org_id in the provider block or FORUMULATE_ORG_ID.")
		return
	}
	// No token is allowed: the forumulate_status_page data source reads the
	// public page. Every managed resource needs one and says so on apply.
	c := client.New(pick(cfg.APIURL, "FORUMULATE_API_URL", client.DefaultAPIURL), token, orgID)
	c.UserAgent = "terraform-provider-forumulate/" + p.version
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *ForumulateProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewStatusComponentResource,
		NewStatusGroupsResource,
		NewStatusIncidentTemplateResource,
		NewStatusMetricResource,
		NewSpaceResource,
		NewSpaceLocaleResource,
		NewSpaceSettingsResource,
		NewCollectionResource,
		NewPageResource,
		NewForumCategoryResource,
		NewDomainResource,
		NewWebhookResource,
		NewSiteResource,
		NewTranslationOverrideResource,
		NewOrgSettingResource,
		NewServiceConnectorResource,
		NewConnectorResource,
		NewSiteManifestResource,
	}
}

func (p *ForumulateProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewStatusPageDataSource, NewSpaceDataSource, NewSpaceTypesDataSource, NewOrgDataSource}
}
