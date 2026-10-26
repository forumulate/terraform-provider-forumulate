package provider

import (
	"context"
	"net/http"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type webhookModel struct {
	ID               types.String `tfsdk:"id"`
	URL              types.String `tfsdk:"url"`
	Events           types.Set    `tfsdk:"events"`
	SpaceID          types.String `tfsdk:"space_id"`
	LibraryID        types.String `tfsdk:"library_id"`
	Format           types.String `tfsdk:"format"`
	Kind             types.String `tfsdk:"kind"`
	CategoryIDs      types.Set    `tfsdk:"category_ids"`
	TagIDs           types.Set    `tfsdk:"tag_ids"`
	GroupIDs         types.Set    `tfsdk:"group_ids"`
	ContentTypes     types.Set    `tfsdk:"content_types"`
	Locales          types.Set    `tfsdk:"locales"`
	BodyEncoding     types.String `tfsdk:"body_encoding"`
	SignatureVersion types.String `tfsdk:"signature_version"`
	Active           types.Bool   `tfsdk:"active"`
	AlertContacts    types.Set    `tfsdk:"alert_contacts"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ExternalRef      types.String `tfsdk:"external_ref"`
	Secret           types.String `tfsdk:"secret"`
	State            types.String `tfsdk:"state"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

var webhookScopes = []fieldMap{
	{"category_ids", "categoryIds", kStrSet}, {"tag_ids", "tagIds", kStrSet}, {"group_ids", "groupIds", kStrSet},
	{"content_types", "contentTypes", kStrSet}, {"locales", "locales", kStrSet},
}

// NewWebhookResource is `forumulate_webhook`.
func NewWebhookResource() resource.Resource {
	scope := func(desc string, max int, each ...validator.String) schema.SetAttribute {
		return schema.SetAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Set{setOf(0, max, each...)},
			MarkdownDescription: desc + " Leave out for no filter (the endpoint hears the event wherever it happens)."}
	}
	return newResource(&resourceSpec[webhookModel]{
		suffix: "webhook",
		what:   "webhook",
		description: "An organization webhook: an HTTPS endpoint that is sent signed events (`page.published`, `topic.created`, ...). It can be confined to one space or content library, narrowed by category, tag, group, content type and locale, " +
			"and told whom to email when it keeps failing.\n\n" +
			"The signing secret is minted by the API once, at creation, and kept in the sensitive `secret` attribute. It is not recoverable afterwards: an imported webhook has no `secret`.",
		importHelp: "the webhook's id",
		paramAlias: map[string]string{"alertContacts": "alert_contacts", "alert_contacts": "alert_contacts", "spaceId": "space_id", "libraryId": "library_id", "bodyEncoding": "body_encoding", "signatureVersion": "signature_version", "externalRef": "external_ref"},
		attrs: map[string]schema.Attribute{
			"id":  computedID("The webhook's id (a UUID)."),
			"url": reqStr("Where deliveries are POSTed: an absolute https URL, up to 2048 characters.", nil, httpURL(2048, false)),
			"events": schema.SetAttribute{Required: true, ElementType: types.StringType, Validators: []validator.Set{setOf(1, 0, oneOf(webhookEvents...))},
				MarkdownDescription: "The events this endpoint hears, by name, at least one. `GET /webhooks/events` publishes the whole vocabulary with a sample payload for each."},
			"space_id": schema.StringAttribute{Optional: true, Validators: []validator.String{uuid()},
				MarkdownDescription: "Confine the endpoint to one space (its id). Leave out for organization-wide, including spaces created later. Not together with `library_id`."},
			"library_id": schema.StringAttribute{Optional: true, Validators: []validator.String{uuid()},
				MarkdownDescription: "Confine the endpoint to one content library: `content.*` events from it only. Not together with `space_id`."},
			"format": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: append(append([]planmodifier.String{}, keepStr...), replStr...), Validators: []validator.String{oneOf("json", "slack", "discord")},
				MarkdownDescription: "The body shape: `json` (the default), or `slack` / `discord` for an incoming-webhook URL from either. Changing it replaces the webhook."},
			"kind": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: append(append([]planmodifier.String{}, keepStr...), replStr...), Validators: []validator.String{oneOf("webhook", "deploy_hook")},
				MarkdownDescription: "`webhook` (the default) delivers signed events; `deploy_hook` calls a Vercel, Netlify or Cloudflare Pages deploy-hook URL on `content.*` events, unsigned and debounced. Changing it replaces the webhook."},
			"category_ids":  scope("Only events naming one of these forum categories (up to 100 category ids).", 100, uuid()),
			"tag_ids":       scope("Only events carrying one of these tags (by name, up to 100, 60 characters each).", 100, length(1, 60)),
			"group_ids":     scope("Only group, user and badge events for these groups (up to 100 group ids).", 100, uuid()),
			"content_types": scope("Only `content.*` events for these content types (by key, up to 100).", 100, length(1, 64)),
			"locales":       scope("Only `content.*` events in these locales (codes, or `default`; up to 100).", 100, length(1, 35)),
			"body_encoding": optComputedStr("`json` (the default) or `form` for a receiver that cannot parse JSON bodies.", oneOf("json", "form")),
			"signature_version": optComputedStr("`v1` (the default for a new endpoint) is the timestamped signature; `both` adds the legacy replayable body-only one for receivers not yet migrated.",
				oneOf("both", "v1")),
			"active": optComputedBool("Whether deliveries are sent. Set `false` to pause an endpoint while keeping its history and secret."),
			"alert_contacts": schema.SetAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Set{setOf(0, 10, length(1, 254), email())},
				MarkdownDescription: "Addresses emailed when the endpoint keeps failing (at 1 hour, at 24 hours, and when it is switched off). Up to 10. Leave out for the organization's owners."},
			"metadata": schema.MapAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Map{metadataMap()},
				MarkdownDescription: "Free-form flat tags. Replaced whole when changed."},
			"external_ref": schema.StringAttribute{Optional: true, Validators: externalRef(), PlanModifiers: replStr,
				MarkdownDescription: "Your own reference, fixed at creation: a repeat create naming it answers the existing webhook (without its secret)."},
			"secret": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keepStr,
				MarkdownDescription: "The signing secret, shown by the API once at creation. Sensitive. Empty for an imported webhook."},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "`healthy`, `failing` or `disabled`."},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: "When the webhook was registered."},
		},
		validate: func(ctx context.Context, cfg *webhookModel, d *diag.Diagnostics) {
			if known(cfg.SpaceID) && known(cfg.LibraryID) {
				d.AddAttributeError(attrPath("library_id"), "Conflicting scope", "A webhook is confined to a space or to a content library, not both.")
			}
			if cfg.Kind.ValueString() == "deploy_hook" && known(cfg.Format) && cfg.Format.ValueString() != "json" {
				d.AddAttributeError(attrPath("format"), "Not available on a deploy hook", "A deploy_hook posts to a deploy-hook URL and has no slack or discord body.")
			}
		},
		create: func(ctx context.Context, c *client.Client, m *webhookModel, d *diag.Diagnostics) error {
			body := Object{"url": m.URL.ValueString(), "events": elementsStrings(m.Events)}
			setStr(body, "spaceId", m.SpaceID, false)
			setStr(body, "libraryId", m.LibraryID, false)
			setStr(body, "format", m.Format, false)
			setStr(body, "kind", m.Kind, false)
			setStr(body, "bodyEncoding", m.BodyEncoding, false)
			setStr(body, "signatureVersion", m.SignatureVersion, false)
			setStr(body, "externalRef", m.ExternalRef, false)
			setStrMap(body, "metadata", m.Metadata, false)
			for _, f := range webhookScopes {
				if v, ok := attrFields(m)[f.tf]; ok {
					if s, ok := valueOf(v).(types.Set); ok && known(s) {
						body[f.wire] = elementsStrings(s)
					}
				}
			}
			out, err := c.DoObject(ctx, http.MethodPost, c.OrgPath("webhooks"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			wh := client.Unwrap(out, "webhook")
			m.ID = getS(wh, "id")
			if secret, ok := out["secret"].(string); ok {
				m.Secret = types.StringValue(secret)
			} else {
				m.Secret = types.StringNull()
			}
			// alertContacts, and active=false, only exist on the update body.
			patch := Object{}
			if known(m.AlertContacts) {
				patch["alertContacts"] = elementsStrings(m.AlertContacts)
			}
			if known(m.Active) && !m.Active.ValueBool() {
				patch["active"] = false
			}
			if len(patch) > 0 {
				pout, err := c.DoObject(ctx, http.MethodPatch, c.OrgPath("webhooks", m.ID.ValueString()), patch)
				if err != nil {
					return err
				}
				warnList(d, pout)
				wh = client.Unwrap(pout, "webhook")
			}
			applyWebhook(m, wh)
			return nil
		},
		created: func(m *webhookModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *webhookModel, d *diag.Diagnostics) error {
			row, err := findRow(ctx, c, c.OrgPath("webhooks"), "webhooks", "id", m.ID.ValueString(), "webhook")
			if err != nil {
				return err
			}
			if m.Secret.IsUnknown() {
				m.Secret = types.StringNull()
			}
			applyWebhook(m, row)
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *webhookModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			body := Object{"url": plan.URL.ValueString(), "events": elementsStrings(plan.Events)}
			setStr(body, "spaceId", plan.SpaceID, true)
			setStr(body, "libraryId", plan.LibraryID, true)
			setStr(body, "bodyEncoding", plan.BodyEncoding, false)
			setStr(body, "signatureVersion", plan.SignatureVersion, false)
			setBool(body, "active", plan.Active, false)
			if known(plan.AlertContacts) || known(prior.AlertContacts) {
				body["alertContacts"] = elementsStrings(plan.AlertContacts)
			}
			if !plan.Metadata.Equal(prior.Metadata) {
				setStrMap(body, "metadata", plan.Metadata, true)
			}
			fp, fo := attrFields(plan), attrFields(prior)
			for _, f := range webhookScopes {
				cur, _ := valueOf(fp[f.tf]).(types.Set)
				old, _ := valueOf(fo[f.tf]).(types.Set)
				switch {
				case known(cur):
					body[f.wire] = elementsStrings(cur)
				case known(old):
					body[f.wire] = nil
				}
			}
			out, err := c.DoObject(ctx, http.MethodPatch, c.OrgPath("webhooks", prior.ID.ValueString()), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			plan.Secret = prior.Secret
			applyWebhook(plan, client.Unwrap(out, "webhook"))
			return nil
		},
		del: func(ctx context.Context, c *client.Client, m *webhookModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, c.OrgPath("webhooks", m.ID.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) { return map[string]string{"id": id}, nil },
	})
}

// emptySetAsNull keeps an unconfigured filter null when the API sends [].
func emptySetAsNull(api, prior types.Set) types.Set {
	if api.IsNull() {
		return api
	}
	if len(api.Elements()) == 0 && !known(prior) {
		return types.SetNull(types.StringType)
	}
	return api
}

func applyWebhook(m *webhookModel, row Object) {
	m.ID = getS(row, "id")
	m.URL = getS(row, "url")
	m.Events = getSS(row, "events")
	m.SpaceID = getS(row, "spaceId")
	m.LibraryID = getS(row, "libraryId")
	m.Format = getS(row, "format")
	m.Kind = getS(row, "kind")
	m.BodyEncoding = getS(row, "bodyEncoding")
	m.SignatureVersion = getS(row, "signatureVersion")
	m.Active = getB(row, "active")
	m.State = getS(row, "state")
	m.CreatedAt = getS(row, "createdAt")
	m.CategoryIDs = emptySetAsNull(getSS(row, "categoryIds"), m.CategoryIDs)
	m.TagIDs = emptySetAsNull(getSS(row, "tagIds"), m.TagIDs)
	m.GroupIDs = emptySetAsNull(getSS(row, "groupIds"), m.GroupIDs)
	m.ContentTypes = emptySetAsNull(getSS(row, "contentTypes"), m.ContentTypes)
	m.Locales = emptySetAsNull(getSS(row, "locales"), m.Locales)
	contacts := getSS(row, "alertContacts")
	if contacts.IsNull() {
		contacts = getSS(row, "alert_contacts")
	}
	m.AlertContacts = emptySetAsNull(contacts, m.AlertContacts)
	if m.Metadata.IsUnknown() {
		m.Metadata = types.MapNull(types.StringType)
	}
	if m.Secret.IsUnknown() {
		m.Secret = types.StringNull()
	}
}
