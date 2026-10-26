package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------------------------------------------------------------------------
// forumulate_translation_override
// ---------------------------------------------------------------------------

// TranslationLocales are the interface languages an override may reword.
var TranslationLocales = []string{"en", "es", "fr", "de", "pt", "ar"}

type translationOverrideModel struct {
	ID     types.String `tfsdk:"id"`
	Locale types.String `tfsdk:"locale"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

// NewTranslationOverrideResource is `forumulate_translation_override`.
func NewTranslationOverrideResource() resource.Resource {
	put := func(ctx context.Context, c *client.Client, m *translationOverrideModel, d *diag.Diagnostics) error {
		out, err := c.DoObject(ctx, http.MethodPut, c.OrgPath("translation-overrides"),
			Object{"locale": m.Locale.ValueString(), "key": m.Key.ValueString(), "value": m.Value.ValueString()})
		if err != nil {
			return err
		}
		warnList(d, out)
		m.ID = types.StringValue(m.Locale.ValueString() + "/" + m.Key.ValueString())
		return nil
	}
	return newResource(&resourceSpec[translationOverrideModel]{
		suffix: "translation_override",
		what:   "translation override",
		description: "One reworded interface string for one language: the reader's \"Sign in\" or \"Search the docs\" in your own words. " +
			"An override replaces the platform's shipped translation for the organization; destroying it returns the shipped wording.",
		importHelp: "<locale>/<key>",
		attrs: map[string]schema.Attribute{
			"id":     computedID("`<locale>/<key>`."),
			"locale": reqStr("The interface language: one of "+joinCodes(TranslationLocales)+". Changing it replaces the override.", replStr, oneOf(TranslationLocales...)),
			"key":    reqStr("The interface string's key (1 to 120 characters). Changing it replaces the override.", replStr, length(1, 120)),
			"value":  reqStr("The wording to show, up to 4000 characters.", nil, length(0, 4000)),
		},
		create:  put,
		created: func(m *translationOverrideModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *translationOverrideModel, d *diag.Diagnostics) error {
			rows, err := c.List(ctx, c.OrgPath("translation-overrides"), "overrides", url.Values{"locale": {m.Locale.ValueString()}})
			if err != nil {
				return err
			}
			for _, r := range rows {
				if getS(r, "key").ValueString() == m.Key.ValueString() && getS(r, "locale").ValueString() == m.Locale.ValueString() {
					m.Value = getS(r, "value")
					m.ID = types.StringValue(m.Locale.ValueString() + "/" + m.Key.ValueString())
					return nil
				}
			}
			return client.NotFound("translation override " + m.Key.ValueString())
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *translationOverrideModel, d *diag.Diagnostics) error {
			return put(ctx, c, plan, d)
		},
		del: func(ctx context.Context, c *client.Client, m *translationOverrideModel, d *diag.Diagnostics) error {
			q := url.Values{"key": {m.Key.ValueString()}, "locale": {m.Locale.ValueString()}}
			return c.Do(ctx, http.MethodDelete, c.OrgPath("translation-overrides")+"?"+q.Encode(), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			loc, key, ok := strings.Cut(id, "/")
			if !ok || loc == "" || key == "" {
				return nil, fmt.Errorf("%q is not <locale>/<key>", id)
			}
			return map[string]string{"locale": loc, "key": key, "id": id}, nil
		},
	})
}

// ---------------------------------------------------------------------------
// forumulate_org_setting
// ---------------------------------------------------------------------------

type orgSettingModel struct {
	ID           types.String `tfsdk:"id"`
	Key          types.String `tfsdk:"key"`
	Value        types.String `tfsdk:"value"`
	Type         types.String `tfsdk:"type"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	DefaultValue types.String `tfsdk:"default_value"`
}

// settingValue turns the configured string into the type the registry
// declares for the key, checking its bounds. The error names `value`.
func settingValue(row Object, raw string) (any, error) {
	bad := func(format string, a ...any) error {
		return &attrError{attr: "value", summary: "Invalid value for " + getS(row, "key").ValueString(), detail: fmt.Sprintf(format, a...)}
	}
	switch getS(row, "type").ValueString() {
	case "boolean":
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, bad("this setting is a boolean: use \"true\" or \"false\" (got %q).", raw)
	case "integer":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, bad("this setting is an integer (got %q).", raw)
		}
		if mn := getF(row, "min"); !mn.IsNull() && float64(n) < mn.ValueFloat64() {
			return nil, bad("must be at least %v (got %d).", mn.ValueFloat64(), n)
		}
		if mx := getF(row, "max"); !mx.IsNull() && float64(n) > mx.ValueFloat64() {
			return nil, bad("must be at most %v (got %d).", mx.ValueFloat64(), n)
		}
		return n, nil
	case "enum":
		opts := stringsOf(row["options"])
		for _, o := range opts {
			if o == raw {
				return raw, nil
			}
		}
		return nil, bad("must be one of: %s (got %q).", strings.Join(opts, ", "), raw)
	}
	if ml := getF(row, "maxLength"); !ml.IsNull() && float64(len(raw)) > ml.ValueFloat64() {
		return nil, bad("must be at most %v characters (got %d).", ml.ValueFloat64(), len(raw))
	}
	if https, _ := row["httpsUrl"].(bool); https && raw != "" && !strings.HasPrefix(raw, "https://") {
		return nil, bad("must be an https:// URL (got %q).", raw)
	}
	return raw, nil
}

func applySetting(m *orgSettingModel, row Object) {
	m.Key = getS(row, "key")
	m.ID = m.Key
	m.Type = getS(row, "type")
	m.IsDefault = getB(row, "isDefault")
	m.Value = getS(row, "value")
	m.DefaultValue = getS(row, "default")
}

// NewOrgSettingResource is `forumulate_org_setting`.
func NewOrgSettingResource() resource.Resource {
	write := func(ctx context.Context, c *client.Client, m *orgSettingModel, d *diag.Diagnostics) error {
		path := c.OrgPath("settings", m.Key.ValueString())
		out, err := c.DoObject(ctx, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		row := client.Unwrap(out, "setting")
		v, err := settingValue(row, m.Value.ValueString())
		if err != nil {
			return err
		}
		out, err = c.DoObject(ctx, http.MethodPut, path, Object{"value": v})
		if err != nil {
			return err
		}
		warnList(d, out)
		applySetting(m, client.Unwrap(out, "setting"))
		// An explicit value is kept as the user spelled it ("08" and "8").
		m.Value = types.StringValue(m.Value.ValueString())
		return nil
	}
	return newResource(&resourceSpec[orgSettingModel]{
		suffix: "org_setting",
		what:   "organization setting",
		description: "One organization setting from the registry (`GET /orgs/{orgId}/settings` lists every key with its type, bounds and default): the default content language, the trash retention, search behaviour and so on. " +
			"The value is written as a string and converted to the type the key declares, with its bounds checked. Destroying the resource resets the key to its default, which follows the platform if the default ever changes.",
		importHelp: "the setting's key",
		attrs: map[string]schema.Attribute{
			"id":            computedID("The setting's key."),
			"key":           reqStr("The setting's key, as listed by `GET /orgs/{orgId}/settings`. An unknown key is a 404. Changing it replaces the resource.", replStr, length(1, 120)),
			"value":         reqStr("The value, as a string: `true`/`false` for a boolean, digits for an integer, one of the options for an enum.", nil, length(0, 20000)),
			"type":          schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The setting's type: `boolean`, `integer`, `enum` or `string`."},
			"is_default":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the stored value is still the default."},
			"default_value": schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The platform default."},
		},
		create:  write,
		created: func(m *orgSettingModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *orgSettingModel, d *diag.Diagnostics) error {
			out, err := c.DoObject(ctx, http.MethodGet, c.OrgPath("settings", m.Key.ValueString()), nil)
			if err != nil {
				return err
			}
			row := client.Unwrap(out, "setting")
			prior := m.Value
			applySetting(m, row)
			// "8" and "08", "true" and "True" mean the same: keep the spelling.
			if known(prior) {
				if v, err := settingValue(row, prior.ValueString()); err == nil && fmt.Sprint(v) == m.Value.ValueString() {
					m.Value = prior
				}
			}
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *orgSettingModel, d *diag.Diagnostics) error {
			return write(ctx, c, plan, d)
		},
		del: func(ctx context.Context, c *client.Client, m *orgSettingModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.OrgPath("settings", m.Key.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) { return map[string]string{"key": id, "id": id}, nil },
	})
}

// ---------------------------------------------------------------------------
// forumulate_service_connector
// ---------------------------------------------------------------------------

// ServiceConnectorProviders are the providers of GET /orgs/{orgId}/service-connectors.
var ServiceConnectorProviders = []string{"apple_wallet", "docusign", "drata", "google_meet", "google_wallet", "vanta", "zoom"}

type serviceConnectorModel struct {
	ID               types.String `tfsdk:"id"`
	Provider         types.String `tfsdk:"provider_name"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	Values           types.Map    `tfsdk:"values"`
	SecretValues     types.Map    `tfsdk:"secret_values"`
	Name             types.String `tfsdk:"name"`
	Configured       types.Bool   `tfsdk:"configured"`
	Connected        types.Bool   `tfsdk:"connected"`
	ConnectedAccount types.String `tfsdk:"connected_account"`
	SecretsSet       types.Set    `tfsdk:"secrets_set"`
	LastError        types.String `tfsdk:"last_error"`
	CallbackURL      types.String `tfsdk:"callback_url"`
}

// secretKeysOf are the secret fields of a provider, from the catalog.
func secretKeysOf(ctx context.Context, c *client.Client, provider string) (secret, all map[string]bool, err error) {
	rows, err := c.List(ctx, c.OrgPath("service-connectors", "catalog"), "connectors", nil)
	if err != nil {
		return nil, nil, err
	}
	secret, all = map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		if getS(r, "provider").ValueString() != provider {
			continue
		}
		fields, _ := r["fields"].([]any)
		for _, f := range fields {
			o, _ := f.(map[string]any)
			key := getS(o, "key").ValueString()
			all[key] = true
			if k := getS(o, "kind").ValueString(); k == "secret" || k == "secretTextarea" {
				secret[key] = true
			}
		}
	}
	return secret, all, nil
}

func serviceConnectorBody(ctx context.Context, c *client.Client, plan, prior *serviceConnectorModel) (Object, error) {
	secret, all, err := secretKeysOf(ctx, c, plan.Provider.ValueString())
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	put := func(m types.Map, attrName string, wantSecret bool) error {
		for k, e := range m.Elements() {
			s, ok := e.(types.String)
			if !ok || !known(s) {
				continue
			}
			if len(all) > 0 && !all[k] {
				return &attrError{attr: attrName, summary: "Unknown connector field", detail: fmt.Sprintf("%q is not a field of the %s connector.", k, plan.Provider.ValueString())}
			}
			if len(all) > 0 && secret[k] != wantSecret {
				if wantSecret {
					return &attrError{attr: attrName, summary: "Not a secret field", detail: fmt.Sprintf("%q is not a secret field: put it in values.", k)}
				}
				return &attrError{attr: attrName, summary: "Secret field in values", detail: fmt.Sprintf("%q is a secret field: put it in secret_values so it stays sensitive and write-only.", k)}
			}
			values[k] = s.ValueString()
		}
		return nil
	}
	if known(plan.Values) {
		if err := put(plan.Values, "values", false); err != nil {
			return nil, err
		}
	}
	if known(plan.SecretValues) {
		if err := put(plan.SecretValues, "secret_values", true); err != nil {
			return nil, err
		}
	}
	if prior != nil {
		for _, pair := range []struct{ old, cur types.Map }{{prior.Values, plan.Values}, {prior.SecretValues, plan.SecretValues}} {
			if known(pair.old) {
				for k := range pair.old.Elements() {
					if _, still := pair.cur.Elements()[k]; !still || pair.cur.IsNull() {
						values[k] = nil
					}
				}
			}
		}
	}
	body := Object{"values": values}
	if known(plan.Enabled) {
		body["enabled"] = plan.Enabled.ValueBool()
	}
	return body, nil
}

func applyServiceConnector(m *serviceConnectorModel, row Object) {
	m.Provider = getS(row, "provider")
	m.ID = m.Provider
	m.Name = getS(row, "name")
	m.Enabled = getB(row, "enabled")
	m.Configured = getB(row, "configured")
	m.Connected = getB(row, "connected")
	m.ConnectedAccount = getS(row, "connectedAccount")
	m.LastError = getS(row, "lastError")
	m.CallbackURL = getS(row, "callbackUrl")
	m.SecretsSet = getSS(row, "secretsSet")
	if known(m.Values) {
		api, _ := row["values"].(map[string]any)
		vals := map[string]attr.Value{}
		for k := range m.Values.Elements() {
			if v, ok := api[k]; ok {
				if s, ok := scalarString(v); ok {
					vals[k] = types.StringValue(s)
				}
			}
		}
		m.Values, _ = types.MapValue(types.StringType, vals)
	}
	if m.SecretValues.IsUnknown() {
		m.SecretValues = types.MapNull(types.StringType)
	}
}

// NewServiceConnectorResource is `forumulate_service_connector`.
func NewServiceConnectorResource() resource.Resource {
	write := func(verb string) func(ctx context.Context, c *client.Client, plan, prior *serviceConnectorModel, d *diag.Diagnostics) error {
		return func(ctx context.Context, c *client.Client, plan, prior *serviceConnectorModel, d *diag.Diagnostics) error {
			body, err := serviceConnectorBody(ctx, c, plan, prior)
			if err != nil {
				return err
			}
			out, err := c.DoObject(ctx, http.MethodPut, c.OrgPath("service-connectors", plan.Provider.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			applyServiceConnector(plan, out)
			return nil
		}
	}
	return newResource(&resourceSpec[serviceConnectorModel]{
		suffix: "service_connector",
		what:   "service connector",
		description: "An organization-level service connector (e-signature, meetings, wallet passes, compliance): its `enabled` switch, readable `values` and write-only `secret_values`.\n\n" +
			"Secret fields (API keys, client secrets, private keys) go in the sensitive `secret_values` map. The API never returns them, only the names of the ones set (`secrets_set`), so drift in a secret cannot be detected; " +
			"a secret you remove from the map is cleared on the next apply. Fields that need an OAuth hand-off (`zoom`, `google_meet`) are connected in the console after the keys are set: `connected` reports it.\n\n" +
			"Destroying the resource disconnects the connector and deletes its stored credentials.",
		importHelp: "the provider name, such as zoom",
		attrs: map[string]schema.Attribute{
			"id":            computedID("The provider name."),
			"provider_name": reqStr("The connector: one of "+joinCodes(ServiceConnectorProviders)+". Changing it replaces the resource.", replStr, oneOf(ServiceConnectorProviders...)),
			"enabled":       optComputedBool("Whether the connector is on. Defaults to on when first saved."),
			"values": schema.MapAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Map{mapCheck{maxVal: 20000, maxLen: 64}},
				MarkdownDescription: "The connector's readable fields (client ids, regions, account ids) by key; see `GET /orgs/{orgId}/service-connectors/catalog`. Only the keys you declare are compared on read."},
			"secret_values": schema.MapAttribute{Optional: true, Sensitive: true, ElementType: types.StringType, Validators: []validator.Map{mapCheck{maxVal: 20000, maxLen: 64}},
				MarkdownDescription: "The connector's secret fields by key. Sensitive and write-only: the API reports only which are set (`secrets_set`)."},
			"name":              schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The connector's display name."},
			"configured":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether every required field is set."},
			"connected":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether an OAuth connector has been connected to an account (null for a key-only connector)."},
			"connected_account": schema.StringAttribute{Computed: true, MarkdownDescription: "The connected account, when there is one."},
			"secrets_set":       schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "The names of the secret fields that have a stored value."},
			"last_error":        schema.StringAttribute{Computed: true, MarkdownDescription: "The last error the connector reported."},
			"callback_url":      schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "The OAuth callback URL to register with the provider, for connectors that use one."},
		},
		create: func(ctx context.Context, c *client.Client, m *serviceConnectorModel, d *diag.Diagnostics) error {
			return write("creating")(ctx, c, m, nil, d)
		},
		created: func(m *serviceConnectorModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *serviceConnectorModel, d *diag.Diagnostics) error {
			out, err := c.DoObject(ctx, http.MethodGet, c.OrgPath("service-connectors", m.Provider.ValueString()), nil)
			if err != nil {
				return err
			}
			// A never-saved connector reads as an unconfigured stub, not a 404:
			// it is "gone" for Terraform when nothing was ever stored.
			if upd, _ := out["updatedAt"].(string); upd == "" && out["updatedAt"] == nil {
				return client.NotFound("service connector " + m.Provider.ValueString())
			}
			applyServiceConnector(m, out)
			return nil
		},
		update: write("updating"),
		del: func(ctx context.Context, c *client.Client, m *serviceConnectorModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.OrgPath("service-connectors", m.Provider.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			return map[string]string{"provider_name": id, "id": id}, nil
		},
	})
}

// ---------------------------------------------------------------------------
// forumulate_space_locale
// ---------------------------------------------------------------------------

type spaceLocaleModel struct {
	ID        types.String `tfsdk:"id"`
	SpaceID   types.String `tfsdk:"space_id"`
	Code      types.String `tfsdk:"code"`
	Label     types.String `tfsdk:"label"`
	IsDefault types.Bool   `tfsdk:"is_default"`
}

// NewSpaceLocaleResource is `forumulate_space_locale`.
func NewSpaceLocaleResource() resource.Resource {
	apply := func(m *spaceLocaleModel, row Object) {
		m.Code = getS(row, "code")
		m.Label = getS(row, "label")
		m.IsDefault = getB(row, "isDefault")
		m.ID = types.StringValue(m.SpaceID.ValueString() + "/" + m.Code.ValueString())
	}
	find := func(ctx context.Context, c *client.Client, m *spaceLocaleModel) (Object, error) {
		return findRow(ctx, c, c.SpacePath(m.SpaceID.ValueString(), "docs", "locales"), "locales", "code", m.Code.ValueString(), "language "+m.Code.ValueString())
	}
	return newResource(&resourceSpec[spaceLocaleModel]{
		suffix: "space_locale",
		what:   "space language",
		description: "A language a docs space publishes in. Adding one gives every page a translatable twin at a locale-prefixed address (`/docs/es/...`) and is the prerequisite for hand-written and machine translations. " +
			"The code `default` relabels the source language (shown as \"Original\" until you name it); it exists already, so it is only renamed, and destroying it changes nothing.",
		importHelp: "<space_id>/<code>",
		attrs: map[string]schema.Attribute{
			"id":       computedID("`<space_id>/<code>`."),
			"space_id": reqStr("The docs space (its id or slug). Changing it replaces the resource.", replStr, idOrSlug()),
			"code": reqStr("The language code (2 to 20 characters: lowercase letters, digits and hyphens, like `es` or `pt-br`), or `default`. It is the translation's address, so changing it replaces the resource.", replStr,
				length(2, 20), pattern(localeOrDefaultRE, "must be a language code like es or pt-br, or default.")),
			"label":      reqStr("The name shown in the reader's language switcher, 1 to 80 characters.", nil, length(1, 80)),
			"is_default": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the space's source language."},
		},
		create: func(ctx context.Context, c *client.Client, m *spaceLocaleModel, d *diag.Diagnostics) error {
			base := c.SpacePath(m.SpaceID.ValueString(), "docs", "locales")
			var err error
			if m.Code.ValueString() == "default" {
				err = c.Do(ctx, http.MethodPatch, base+"/default", Object{"label": m.Label.ValueString()}, nil)
			} else {
				err = c.Do(ctx, http.MethodPost, base, Object{"code": m.Code.ValueString(), "label": m.Label.ValueString()}, nil)
			}
			if err != nil {
				return err
			}
			m.ID = types.StringValue(m.SpaceID.ValueString() + "/" + m.Code.ValueString())
			row, err := find(ctx, c, m)
			if err != nil {
				return err
			}
			apply(m, row)
			return nil
		},
		created: func(m *spaceLocaleModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *spaceLocaleModel, d *diag.Diagnostics) error {
			row, err := find(ctx, c, m)
			if err != nil {
				return err
			}
			apply(m, row)
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *spaceLocaleModel, d *diag.Diagnostics) error {
			if err := c.Do(ctx, http.MethodPatch, c.SpacePath(plan.SpaceID.ValueString(), "docs", "locales", plan.Code.ValueString()), Object{"label": plan.Label.ValueString()}, nil); err != nil {
				return err
			}
			row, err := find(ctx, c, plan)
			if err != nil {
				return err
			}
			apply(plan, row)
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *spaceLocaleModel, d *diag.Diagnostics) error {
			if m.Code.ValueString() == "default" {
				return nil
			}
			return c.Do(ctx, http.MethodDelete, c.SpacePath(m.SpaceID.ValueString(), "docs", "locales", m.Code.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 2)
			if err != nil {
				return nil, err
			}
			return map[string]string{"space_id": p[0], "code": p[1], "id": id}, nil
		},
	})
}
