package provider

import (
	"context"
	"net/http"
	"regexp"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type connectorModel struct {
	ID               types.String `tfsdk:"id"`
	SpaceID          types.String `tfsdk:"space_id"`
	ProviderID       types.String `tfsdk:"provider_id"`
	Config           types.Map    `tfsdk:"config"`
	EmbedEnabled     types.Bool   `tfsdk:"embed_enabled"`
	KnowledgeEnabled types.Bool   `tfsdk:"knowledge_enabled"`
	KnowledgeScopes  types.Set    `tfsdk:"knowledge_scopes"`
	Appearance       types.Object `tfsdk:"appearance"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ExternalRef      types.String `tfsdk:"external_ref"`
	MintToken        types.Bool   `tfsdk:"mint_token"`
	TokenRotation    types.String `tfsdk:"token_rotation"`
	Token            types.String `tfsdk:"token"`
	TokenPrefix      types.String `tfsdk:"token_prefix"`
	ProviderName     types.String `tfsdk:"provider_name"`
	ConfiguredFields types.Set    `tfsdk:"configured_fields"`
	MissingFields    types.Set    `tfsdk:"missing_fields"`
}

var connectorAppearanceFields = []fieldMap{
	{"primary_color", "primaryColor", kStr}, {"color_scheme", "colorScheme", kStr}, {"position", "position", kStr}, {"greeting", "greeting", kStr},
}

var knowledgeScopes = []string{"chunks", "search", "ask", "llms", "questions", "suggestions"}

var connectorConfigKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

// NewConnectorResource is `forumulate_connector`: a chat or AI product
// connected to one space (what the manifest's `connectors` block installs).
func NewConnectorResource() resource.Resource {
	set := func(rows Object, m *connectorModel) {
		m.ID = getS(rows, "id")
		m.ProviderID = getS(rows, "providerId")
		m.ProviderName = getS(rows, "providerName")
		m.EmbedEnabled = getB(rows, "embedEnabled")
		m.KnowledgeEnabled = getB(rows, "knowledgeEnabled")
		m.KnowledgeScopes = getSS(rows, "knowledgeScopes")
		m.TokenPrefix = getS(rows, "tokenPrefix")
		m.ConfiguredFields = getSS(rows, "configuredFields")
		m.MissingFields = getSS(rows, "missingFields")
		app, _ := rows["appearance"].(map[string]any)
		m.Appearance = objectFromAPI(m.Appearance, connectorAppearanceFields, app, false)
		if m.Metadata.IsUnknown() {
			m.Metadata = types.MapNull(types.StringType)
		}
		if m.Token.IsUnknown() {
			m.Token = types.StringNull()
		}
	}
	mint := func(ctx context.Context, c *client.Client, m *connectorModel) error {
		out, err := c.DoObject(ctx, http.MethodPost, c.SpacePath(m.SpaceID.ValueString(), "connectors", m.ID.ValueString(), "token"), Object{})
		if err != nil {
			return err
		}
		m.Token = getS(out, "token")
		m.TokenPrefix = getS(out, "tokenPrefix")
		return nil
	}
	find := func(ctx context.Context, c *client.Client, m *connectorModel) (Object, error) {
		return findRow(ctx, c, c.SpacePath(m.SpaceID.ValueString(), "connectors"), "connectors", "id", m.ID.ValueString(), "connector")
	}
	return newResource(&resourceSpec[connectorModel]{
		suffix: "connector",
		what:   "connector",
		description: "A chat or AI product connected to one space (Intercom, Zendesk, a custom provider...): installed with its `config`, optionally embedded on the reader and/or given read access to the space's knowledge base. " +
			"`GET /orgs/{orgId}/connector-providers` lists the providers and the config fields each needs. For the organization-level e-signature, meeting and wallet connectors see `forumulate_service_connector`.\n\n" +
			"`config` is sensitive and write-only: the API reports only which fields are set (`configured_fields`) and which are still missing (`missing_fields`), so a changed secret cannot be detected. " +
			"With `mint_token = true` the connector gets an access token for the product to read the knowledge base with; it is shown once and kept, sensitive, in `token`. Change `token_rotation` to mint a new one.",
		importHelp: "<space_id>/<connector_id>",
		paramAlias: map[string]string{"providerId": "provider_id", "embedEnabled": "embed_enabled", "knowledgeEnabled": "knowledge_enabled", "knowledgeScopes": "knowledge_scopes", "externalRef": "external_ref"},
		attrs: map[string]schema.Attribute{
			"id":          computedID("The connector's id (a UUID)."),
			"space_id":    reqStr("The space (its id or slug). Changing it replaces the connector.", replStr, idOrSlug()),
			"provider_id": reqStr("The provider (2 to 70 characters), such as `intercom`; see `GET /orgs/{orgId}/connector-providers`. Changing it replaces the connector.", replStr, length(2, 70)),
			"config": schema.MapAttribute{Optional: true, Sensitive: true, ElementType: types.StringType,
				Validators:          []validator.Map{mapCheck{keyRE: connectorConfigKey, keyMsg: "must start with a letter and use letters, digits and . _ -", maxVal: 2000}},
				MarkdownDescription: "The provider's configuration fields by key, each up to 2000 characters. Sensitive and write-only; a key you remove is cleared."},
			"embed_enabled":     optComputedBool("Load the provider's widget on the reader. Defaults to off."),
			"knowledge_enabled": optComputedBool("Let the provider read the space's knowledge base (see `knowledge_scopes`). Defaults to off."),
			"knowledge_scopes": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators:          []validator.Set{setOf(0, 0, oneOf(knowledgeScopes...))},
				MarkdownDescription: "What the provider may use of the knowledge base: any of `chunks`, `search`, `ask`, `llms`, `questions`, `suggestions`."},
			"appearance": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "How the embedded widget looks, for the providers that support it. Only the attributes you set are managed.",
				Attributes: map[string]schema.Attribute{
					"primary_color": schema.StringAttribute{Optional: true, Validators: []validator.String{hexColor()}, MarkdownDescription: "The widget colour, six-digit hex with the leading `#`."},
					"color_scheme":  schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("system", "light", "dark")}, MarkdownDescription: "`system`, `light` or `dark`."},
					"position":      schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("left", "right")}, MarkdownDescription: "`left` or `right`."},
					"greeting":      schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 200)}, MarkdownDescription: "The widget's greeting, up to 200 characters."},
				},
			},
			"metadata": schema.MapAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Map{metadataMap()},
				MarkdownDescription: "Free-form tags. Replaced whole when changed. Not returned on a read, so drift is not detected."},
			"external_ref": schema.StringAttribute{Optional: true, Validators: externalRef(), PlanModifiers: replStr, MarkdownDescription: "Your own reference, fixed at creation."},
			"mint_token":   schema.BoolAttribute{Optional: true, MarkdownDescription: "Mint an access token for the connected product. Turning it off revokes the token."},
			"token_rotation": schema.StringAttribute{Optional: true,
				MarkdownDescription: "Any string; change it to mint a new token (the old one stops working). Only meaningful with `mint_token`."},
			"token":             schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keepStr, MarkdownDescription: "The access token, shown by the API once when minted. Sensitive."},
			"token_prefix":      schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The first characters of the token, to recognise it in the console."},
			"provider_name":     schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The provider's display name."},
			"configured_fields": schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "The names of the config fields that have a stored value."},
			"missing_fields":    schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "The names of required config fields still missing; the connector is incomplete until this is empty."},
		},
		validate: func(ctx context.Context, cfg *connectorModel, d *diag.Diagnostics) {
			if known(cfg.TokenRotation) && !cfg.MintToken.ValueBool() && !cfg.MintToken.IsUnknown() {
				d.AddAttributeError(attrPath("token_rotation"), "token_rotation needs mint_token", "token_rotation only has an effect with mint_token = true.")
			}
		},
		create: func(ctx context.Context, c *client.Client, m *connectorModel, d *diag.Diagnostics) error {
			body := Object{"providerId": m.ProviderID.ValueString()}
			setStrMap(body, "config", m.Config, false)
			setBool(body, "embedEnabled", m.EmbedEnabled, false)
			setBool(body, "knowledgeEnabled", m.KnowledgeEnabled, false)
			setStrSet(body, "knowledgeScopes", m.KnowledgeScopes, false)
			if known(m.Appearance) {
				body["appearance"] = objectBody(m.Appearance, connectorAppearanceFields, nil)
			}
			setStr(body, "externalRef", m.ExternalRef, false)
			setStrMap(body, "metadata", m.Metadata, false)
			out, err := c.DoObject(ctx, http.MethodPost, c.SpacePath(m.SpaceID.ValueString(), "connectors"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			row := client.Unwrap(out, "connector")
			m.ID = getS(row, "id")
			m.Token = types.StringNull()
			set(row, m)
			if m.MintToken.ValueBool() {
				if err := mint(ctx, c, m); err != nil {
					return err
				}
			}
			if mf := m.MissingFields; known(mf) && len(mf.Elements()) > 0 {
				d.AddWarning("The connector is incomplete", "Fields still missing: "+joinSet(mf)+". Add them to config.")
			}
			return nil
		},
		created: func(m *connectorModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *connectorModel, d *diag.Diagnostics) error {
			row, err := find(ctx, c, m)
			if err != nil {
				return err
			}
			set(row, m)
			if has, ok := row["hasToken"].(bool); ok && !has && m.MintToken.ValueBool() {
				// Revoked outside Terraform: show the difference so the next apply mints again.
				m.MintToken = types.BoolValue(false)
				m.Token = types.StringNull()
			}
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *connectorModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			body := Object{}
			setBool(body, "embedEnabled", plan.EmbedEnabled, false)
			setBool(body, "knowledgeEnabled", plan.KnowledgeEnabled, false)
			setStrSet(body, "knowledgeScopes", plan.KnowledgeScopes, false)
			if known(plan.Appearance) || known(prior.Appearance) {
				body["appearance"] = objectBody(plan.Appearance, connectorAppearanceFields, &prior.Appearance)
			}
			if !plan.Config.Equal(prior.Config) {
				cfg := map[string]any{}
				if known(plan.Config) {
					for k, e := range plan.Config.Elements() {
						if s, ok := e.(types.String); ok {
							cfg[k] = s.ValueString()
						}
					}
				}
				for k := range prior.Config.Elements() {
					if _, still := cfg[k]; !still {
						cfg[k] = nil
					}
				}
				body["config"] = cfg
			}
			if !plan.Metadata.Equal(prior.Metadata) {
				setStrMap(body, "metadata", plan.Metadata, true)
			}
			out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(plan.SpaceID.ValueString(), "connectors", prior.ID.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			set(client.Unwrap(out, "connector"), plan)
			// The token is settled after the connector's own fields; the plan
			// marked token and token_prefix unknown when it is going to change.
			if plan.Token.IsNull() || plan.Token.IsUnknown() {
				plan.Token = prior.Token
			}
			switch {
			case plan.MintToken.ValueBool() && (!prior.MintToken.ValueBool() || !plan.TokenRotation.Equal(prior.TokenRotation)):
				if err := mint(ctx, c, plan); err != nil {
					return err
				}
			case !plan.MintToken.ValueBool() && prior.MintToken.ValueBool():
				if err := c.Do(ctx, http.MethodDelete, c.SpacePath(plan.SpaceID.ValueString(), "connectors", prior.ID.ValueString(), "token"), nil, nil); err != nil {
					return err
				}
				plan.Token = types.StringNull()
			}
			return nil
		},
		modifyPlan: func(ctx context.Context, c *client.Client, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
			// A token that will be minted or revoked is unknown until the apply.
			var plan, state connectorModel
			if req.State.Raw.IsNull() || req.Plan.Get(ctx, &plan).HasError() || req.State.Get(ctx, &state).HasError() {
				return
			}
			minting := plan.MintToken.ValueBool() && (!state.MintToken.ValueBool() || !plan.TokenRotation.Equal(state.TokenRotation))
			revoking := !plan.MintToken.ValueBool() && state.MintToken.ValueBool()
			if minting || revoking {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("token"), types.StringUnknown())...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("token_prefix"), types.StringUnknown())...)
			}
		},
		del: func(ctx context.Context, c *client.Client, m *connectorModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.SpacePath(m.SpaceID.ValueString(), "connectors", m.ID.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 2)
			if err != nil {
				return nil, err
			}
			return map[string]string{"space_id": p[0], "id": p[1]}, nil
		},
	})
}

func joinSet(s types.Set) string {
	out := ""
	for i, e := range s.Elements() {
		if v, ok := e.(types.String); ok {
			if i > 0 {
				out += ", "
			}
			out += v.ValueString()
		}
	}
	return out
}
