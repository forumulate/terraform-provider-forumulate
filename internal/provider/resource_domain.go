package provider

import (
	"context"
	"net/http"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type domainModel struct {
	ID                 types.String `tfsdk:"id"`
	SpaceID            types.String `tfsdk:"space_id"`
	SiteID             types.String `tfsdk:"site_id"`
	Hostname           types.String `tfsdk:"hostname"`
	BasePath           types.String `tfsdk:"base_path"`
	AlsoRedirectWWW    types.Bool   `tfsdk:"also_redirect_www"`
	ExternalRef        types.String `tfsdk:"external_ref"`
	Metadata           types.Map    `tfsdk:"metadata"`
	RedirectTo         types.String `tfsdk:"redirect_to"`
	IPAllowList        types.Set    `tfsdk:"ip_allow_list"`
	Activate           types.Bool   `tfsdk:"activate"`
	Verified           types.Bool   `tfsdk:"verified"`
	Active             types.Bool   `tfsdk:"active"`
	DNSTarget          types.Object `tfsdk:"dns_target"`
	VerificationRecord types.Object `tfsdk:"verification_record"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

var dnsFieldDocs = map[string]string{"type": "The DNS record type (`A`, `CNAME`).", "name": "The record's host name.", "value": "The record's value."}

var dnsTargetTypes = map[string]attr.Type{"type": types.StringType, "name": types.StringType, "value": types.StringType}
var verificationTypes = map[string]attr.Type{"name": types.StringType, "value": types.StringType}

func (m *domainModel) base(c *client.Client, rest ...string) string {
	if known(m.SiteID) {
		return c.SitePath(m.SiteID.ValueString(), append([]string{"domains"}, rest...)...)
	}
	return c.SpacePath(m.SpaceID.ValueString(), append([]string{"domains"}, rest...)...)
}

func (m *domainModel) isSite() bool { return known(m.SiteID) }

// NewDomainResource is `forumulate_domain`.
func NewDomainResource() resource.Resource {
	nested := func(desc string, keys ...string) schema.SingleNestedAttribute {
		attrs := map[string]schema.Attribute{}
		for _, k := range keys {
			attrs[k] = schema.StringAttribute{Computed: true, MarkdownDescription: dnsFieldDocs[k]}
		}
		return schema.SingleNestedAttribute{Computed: true, Attributes: attrs, MarkdownDescription: desc, PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()}}
	}
	return newResource(&resourceSpec[domainModel]{
		suffix: "domain",
		what:   "domain",
		description: "A custom hostname for a space or a whole site. Creating one claims the name and returns the DNS records to publish (`dns_target`, `verification_record`: feed them to your DNS provider's resource). " +
			"Nothing is served at the name until it is verified and activated: set `activate = true` once the records exist and the next apply verifies and switches traffic over (apply again after DNS propagates if the first attempt warns).\n\n" +
			"Exactly one of `space_id` or `site_id` is required. `redirect_to` and `ip_allow_list` exist on a space's domain only.",
		importHelp: "space/<space_id>/<domain_id> or site/<site_id>/<domain_id>",
		paramAlias: map[string]string{"alsoRedirectWww": "also_redirect_www", "basePath": "base_path", "ipAllowList": "ip_allow_list", "redirectTo": "redirect_to", "externalRef": "external_ref"},
		attrs: map[string]schema.Attribute{
			"id": computedID("The domain's id (a UUID)."),
			"space_id": schema.StringAttribute{Optional: true, PlanModifiers: replStr, Validators: []validator.String{idOrSlug()},
				MarkdownDescription: "The space the hostname serves (its id or slug). Exactly one of `space_id` and `site_id`. Changing it replaces the domain."},
			"site_id": schema.StringAttribute{Optional: true, PlanModifiers: replStr, Validators: []validator.String{idOrSlug()},
				MarkdownDescription: "The site the hostname serves, every member space at one address. Changing it replaces the domain."},
			"hostname":          reqStr("The hostname, such as `docs.example.com` (no scheme or path). Changing it replaces the domain.", replStr, hostname()),
			"base_path":         schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: append(append([]planmodifier.String{}, keepStr...), replStr...), Validators: []validator.String{length(0, 120)}, MarkdownDescription: "Serve under this path of the hostname instead of its root, such as `/docs` (up to 120 characters). Changing it replaces the domain."},
			"also_redirect_www": optComputedBool("Also answer `www.<hostname>` with a permanent redirect here. Use it when your own app does not already redirect www."),
			"external_ref": schema.StringAttribute{Optional: true, PlanModifiers: replStr, Validators: externalRef(),
				MarkdownDescription: "Your own reference, fixed at creation: a repeat create naming it answers the existing domain."},
			"metadata": schema.MapAttribute{Optional: true, ElementType: types.StringType, Validators: []validator.Map{metadataMap()},
				MarkdownDescription: "Tags an orchestrator keeps on the domain. Replaced whole. The API does not return them on a read, so drift is not detected."},
			"redirect_to": schema.StringAttribute{Optional: true, Validators: []validator.String{httpURL(512, false)},
				MarkdownDescription: "Permanently redirect the whole hostname to this URL (for retiring a name). A space domain only."},
			"ip_allow_list": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators:          []validator.Set{setOf(0, 200, length(1, 64))},
				MarkdownDescription: "Only these networks (CIDR ranges or addresses, up to 200) may read the hostname. A space domain only."},
			"activate": schema.BoolAttribute{Optional: true,
				MarkdownDescription: "Verify the DNS records and switch traffic to the hostname. If the records are not there yet the apply warns and the next apply tries again."},
			"verified":            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the DNS records prove ownership."},
			"active":              schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether traffic is served at the hostname."},
			"dns_target":          nested("The record that points the hostname at Forumulate: `type` (`A` or `CNAME`), `name` and `value`.", "type", "name", "value"),
			"verification_record": nested("The TXT record that proves you own the hostname: `name` and `value`.", "name", "value"),
			"created_at":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "When the domain was claimed."},
		},
		validate: func(ctx context.Context, cfg *domainModel, d *diag.Diagnostics) {
			hasSpace, hasSite := cfg.SpaceID.IsNull() == false, cfg.SiteID.IsNull() == false
			if !cfg.SpaceID.IsUnknown() && !cfg.SiteID.IsUnknown() && hasSpace == hasSite {
				d.AddError("Exactly one of space_id and site_id", "A domain serves a space or a whole site: set one of space_id and site_id, not both and not neither.")
			}
			if hasSite && (known(cfg.RedirectTo) || known(cfg.IPAllowList)) {
				d.AddAttributeError(path.Root("site_id"), "Not available on a site domain", "redirect_to and ip_allow_list exist on a space's domain only.")
			}
		},
		create: func(ctx context.Context, c *client.Client, m *domainModel, d *diag.Diagnostics) error {
			body := Object{"hostname": m.Hostname.ValueString()}
			setStr(body, "basePath", m.BasePath, false)
			setBool(body, "alsoRedirectWww", m.AlsoRedirectWWW, false)
			setStr(body, "externalRef", m.ExternalRef, false)
			setStrMap(body, "metadata", m.Metadata, false)
			out, err := c.DoObject(ctx, http.MethodPost, m.base(c), body)
			if err != nil {
				return err
			}
			warnList(d, out)
			dom := client.Unwrap(out, "domain")
			m.ID = getS(dom, "id")
			if known(m.RedirectTo) {
				if err := c.Do(ctx, http.MethodPut, m.base(c, m.ID.ValueString(), "redirect"), Object{"redirectTo": m.RedirectTo.ValueString()}, nil); err != nil {
					return err
				}
			}
			if known(m.IPAllowList) {
				if err := c.Do(ctx, http.MethodPut, m.base(c, m.ID.ValueString(), "ip-allow-list"), Object{"ipAllowList": elementsStrings(m.IPAllowList)}, nil); err != nil {
					return err
				}
			}
			if m.Activate.ValueBool() {
				activateDomain(ctx, c, m, d)
			}
			return refreshDomain(ctx, c, m)
		},
		created: func(m *domainModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *domainModel, d *diag.Diagnostics) error {
			return refreshDomain(ctx, c, m)
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *domainModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			id := prior.ID.ValueString()
			if known(plan.AlsoRedirectWWW) && !plan.AlsoRedirectWWW.Equal(prior.AlsoRedirectWWW) {
				if err := c.Do(ctx, http.MethodPut, plan.base(c, id, "www"), Object{"alsoRedirectWww": plan.AlsoRedirectWWW.ValueBool()}, nil); err != nil {
					return err
				}
			}
			if !plan.RedirectTo.Equal(prior.RedirectTo) {
				body := Object{"redirectTo": nil}
				if known(plan.RedirectTo) {
					body["redirectTo"] = plan.RedirectTo.ValueString()
				}
				if err := c.Do(ctx, http.MethodPut, plan.base(c, id, "redirect"), body, nil); err != nil {
					return err
				}
			}
			if known(plan.IPAllowList) && !plan.IPAllowList.Equal(prior.IPAllowList) {
				if err := c.Do(ctx, http.MethodPut, plan.base(c, id, "ip-allow-list"), Object{"ipAllowList": elementsStrings(plan.IPAllowList)}, nil); err != nil {
					return err
				}
			}
			if !plan.Metadata.Equal(prior.Metadata) {
				body := Object{}
				setStrMap(body, "metadata", plan.Metadata, true)
				if err := c.Do(ctx, http.MethodPatch, plan.base(c, id), body, nil); err != nil {
					return err
				}
			}
			if plan.Activate.ValueBool() && !prior.Active.ValueBool() {
				activateDomain(ctx, c, plan, d)
			}
			return refreshDomain(ctx, c, plan)
		},
		del: func(ctx context.Context, c *client.Client, m *domainModel, d *diag.Diagnostics) error {
			return c.Do(ctx, http.MethodDelete, m.base(c, m.ID.ValueString()), nil, nil)
		},
		importID: func(id string) (map[string]string, error) {
			p, err := splitID(id, 3)
			if err != nil {
				return nil, err
			}
			switch p[0] {
			case "space":
				return map[string]string{"space_id": p[1], "id": p[2]}, nil
			case "site":
				return map[string]string{"site_id": p[1], "id": p[2]}, nil
			}
			return nil, errBadPrefix(p[0])
		},
		modifyPlan: func(ctx context.Context, c *client.Client, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
			// `activate = true` against a domain that is not active yet is work
			// to do on the next apply even though no configured value changed:
			// mark `active` as changing so the plan shows it.
			var plan, state domainModel
			if req.State.Raw.IsNull() || req.Plan.Get(ctx, &plan).HasError() || req.State.Get(ctx, &state).HasError() {
				return
			}
			if plan.Activate.ValueBool() && !state.Active.ValueBool() {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("active"), types.BoolUnknown())...)
			}
		},
	})
}

// activateDomain verifies the DNS records and switches traffic over. A name
// whose records are not published yet is a warning, not a failure: the domain
// is claimed, and the next apply tries again.
func activateDomain(ctx context.Context, c *client.Client, m *domainModel, d *diag.Diagnostics) {
	err := c.Do(ctx, http.MethodPost, m.base(c, m.ID.ValueString(), "verify"), Object{"activate": true}, nil)
	if err == nil {
		return
	}
	if e, ok := client.AsAPIError(err); ok && e.Status >= 400 && e.Status < 500 {
		d.AddWarning("Domain not verified yet",
			"The DNS records for "+m.Hostname.ValueString()+" were not found, so traffic was not switched over: "+e.Message+
				" Publish dns_target and verification_record at your DNS provider, then apply again.")
		return
	}
	d.AddError("Forumulate API: verifying domain", err.Error())
}

type badPrefix string

func (b badPrefix) Error() string {
	return "the id must start with space/ or site/, not \"" + string(b) + "/\""
}
func errBadPrefix(s string) error { return badPrefix(s) }

func refreshDomain(ctx context.Context, c *client.Client, m *domainModel) error {
	row, err := findRow(ctx, c, m.base(c), "domains", "id", m.ID.ValueString(), "domain")
	if err != nil {
		return err
	}
	m.ID = getS(row, "id")
	m.Hostname = getS(row, "hostname")
	bp := getS(row, "basePath")
	m.BasePath = bp
	m.AlsoRedirectWWW = getB(row, "alsoRedirectWww")
	m.Verified = getB(row, "verified")
	m.Active = getB(row, "active")
	m.CreatedAt = getS(row, "createdAt")
	if !m.isSite() {
		m.RedirectTo = nullIfEmpty(getS(row, "redirectTo"), m.RedirectTo)
		m.IPAllowList = getSS(row, "ipAllowList")
	} else {
		m.RedirectTo = types.StringNull()
		m.IPAllowList = types.SetNull(types.StringType)
	}
	m.DNSTarget = objFrom(dnsTargetTypes, row["dnsTarget"])
	m.VerificationRecord = objFrom(verificationTypes, row["verificationRecord"])
	if m.Metadata.IsUnknown() {
		m.Metadata = types.MapNull(types.StringType)
	}
	return nil
}

// objFrom reads an object of string fields (DNS records) into a Terraform object.
func objFrom(at map[string]attr.Type, raw any) types.Object {
	o, ok := raw.(map[string]any)
	if !ok {
		return types.ObjectNull(at)
	}
	vals := map[string]attr.Value{}
	for k := range at {
		vals[k] = getS(o, k)
	}
	obj, _ := types.ObjectValue(at, vals)
	return obj
}
