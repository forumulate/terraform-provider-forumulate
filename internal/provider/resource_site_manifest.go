package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// manifestBlocks are the top-level keys of a version-2 site manifest
// (siteManifestV2Schema in apps/api/src/manifest). The API refuses any other
// key by name, so the provider refuses it at plan time.
var manifestBlocks = []string{
	"version", "spaces", "sources", "domains", "reader_auth", "readerAuth", "consent", "connectors", "executor",
	"webhooks", "status_monitors", "brand", "pages", "sites", "translationOverrides", "orgSettings", "prune",
}

type siteManifestModel struct {
	ID            types.String `tfsdk:"id"`
	ManifestJSON  types.String `tfsdk:"manifest_json"`
	Prune         types.Bool   `tfsdk:"prune"`
	DestroyPrunes types.Bool   `tfsdk:"destroy_prunes"`
	TimeoutSecs   types.Int64  `tfsdk:"timeout_seconds"`
	Pending       types.String `tfsdk:"pending"`
	IDs           types.Map    `tfsdk:"ids"`
	LastApplyJSON types.String `tfsdk:"last_apply_json"`
	ManualJSON    types.String `tfsdk:"manual_json"`
	SecretsJSON   types.String `tfsdk:"secrets_json"`
}

// manifestDoc is the document to send: the configured JSON plus `prune`.
func manifestDoc(m *siteManifestModel) (map[string]any, error) {
	doc, err := parseObject(m.ManifestJSON.ValueString())
	if err != nil {
		return nil, err
	}
	if known(m.Prune) {
		doc["prune"] = m.Prune.ValueBool()
	}
	return doc, nil
}

// manifestStep is one plan or apply step of the answer.
type manifestStep struct {
	Unit     string
	Ref      string
	Action   string
	Status   string
	ID       string
	ReadBack string
	Error    string
	Diff     map[string]any
	Shown    map[string]any
}

func parseSteps(out Object) []manifestStep {
	arr, _ := out["steps"].([]any)
	steps := make([]manifestStep, 0, len(arr))
	for _, e := range arr {
		o, ok := e.(map[string]any)
		if !ok {
			continue
		}
		diff, _ := o["diff"].(map[string]any)
		shown, _ := o["shownOnce"].(map[string]any)
		steps = append(steps, manifestStep{
			Unit: getS(o, "unit").ValueString(), Ref: getS(o, "ref").ValueString(), Action: getS(o, "action").ValueString(),
			Status: getS(o, "status").ValueString(), ID: getS(o, "id").ValueString(), ReadBack: getS(o, "readBack").ValueString(),
			Error: getS(o, "error").ValueString(), Diff: diff, Shown: shown,
		})
	}
	return steps
}

func actionable(s manifestStep) bool {
	return s.Action == "create" || s.Action == "update" || s.Action == "delete"
}

// pendingText is the one-line summary kept in state: "none", or the counts.
func pendingText(steps []manifestStep) string {
	counts := map[string]int{}
	for _, s := range steps {
		if actionable(s) {
			counts[s.Action]++
		}
	}
	if len(counts) == 0 {
		return "none"
	}
	parts := []string{}
	for _, a := range []string{"create", "update", "delete"} {
		if counts[a] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[a], a))
		}
	}
	return strings.Join(parts, ", ")
}

func stepLine(s manifestStep) string {
	line := fmt.Sprintf("%s %s/%s", s.Action, s.Unit, s.Ref)
	if len(s.Diff) > 0 {
		keys := make([]string, 0, len(s.Diff))
		for k := range s.Diff {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if d, ok := s.Diff[k].(map[string]any); ok {
				parts = append(parts, fmt.Sprintf("%s: %s -> %s", k, brief(d["from"]), brief(d["to"])))
			} else {
				parts = append(parts, k)
			}
		}
		line += " (" + strings.Join(parts, "; ") + ")"
	}
	return line
}

func brief(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

func idsOf(steps []manifestStep) types.Map {
	vals := map[string]attr.Value{}
	for _, s := range steps {
		if s.ID != "" && s.Status != "failed" {
			vals[s.Unit+"/"+s.Ref] = types.StringValue(s.ID)
		}
	}
	m, _ := types.MapValue(types.StringType, vals)
	return m
}

// manifestClient is the client with the apply timeout: the apply is one
// request that may run for minutes (domains, large page sets).
func manifestClient(c *client.Client, m *siteManifestModel) *client.Client {
	secs := m.TimeoutSecs.ValueInt64()
	if secs <= 0 {
		secs = 600
	}
	return c.WithTimeout(time.Duration(secs) * time.Second)
}

func manifestPut(ctx context.Context, c *client.Client, doc map[string]any, apply bool) (Object, error) {
	p := c.OrgPath("manifest")
	if apply {
		p += "?apply=true"
	}
	return c.DoObject(ctx, http.MethodPut, p, doc)
}

// manifestDiag pins an API refusal of the document to manifest_json.
func manifestDiag(d *diag.Diagnostics, what string, err error) {
	e, ok := client.AsAPIError(err)
	if !ok {
		d.AddAttributeError(path.Root("manifest_json"), "Forumulate API: "+what, err.Error())
		return
	}
	detail := e.Message
	if e.Param != "" {
		detail = "at " + e.Param + ": " + detail
	}
	if len(e.Details) > 0 {
		keys := make([]string, 0, len(e.Details))
		for k := range e.Details {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			detail += "\n  " + k + ": " + strings.Join(e.Details[k], "; ")
		}
	}
	if e.Hint != "" {
		detail += "\n\nHint: " + e.Hint
	}
	if e.RequestID != "" {
		detail += "\n\nRequest id: " + e.RequestID
	}
	d.AddAttributeError(path.Root("manifest_json"), "Forumulate API: "+what+" ("+e.Code+")", detail)
}

// NewSiteManifestResource is `forumulate_site_manifest`.
func NewSiteManifestResource() resource.Resource {
	run := func(ctx context.Context, c *client.Client, m, prior *siteManifestModel, d *diag.Diagnostics) error {
		doc, err := manifestDoc(m)
		if err != nil {
			return &attrError{attr: "manifest_json", summary: "Invalid manifest", detail: err.Error()}
		}
		mc := manifestClient(c, m)
		out, err := manifestPut(ctx, mc, doc, true)
		if err != nil {
			// The edge cuts a long request at 30 s with a 5xx while the work
			// carries on; the apply is made of idempotent units, so look at
			// what is left rather than guess.
			if ae, ok := client.AsAPIError(err); !ok || ae.Status >= 500 {
				if left, perr := manifestPut(ctx, mc, doc, false); perr == nil {
					steps := parseSteps(left)
					if pendingText(steps) == "none" {
						d.AddWarning("The apply answered with an error, but the site matches the manifest",
							"The request ended with: "+err.Error()+"\nA re-plan finds nothing left to do, so the apply completed.")
						return finishManifest(m, prior, steps, left, d)
					}
				}
			}
			return err
		}
		warnList(d, out)
		steps := parseSteps(out)
		var failed []string
		for _, s := range steps {
			if s.Status == "failed" {
				failed = append(failed, fmt.Sprintf("%s/%s: %s", s.Unit, s.Ref, s.Error))
			}
			if s.ReadBack == "mismatch" {
				d.AddWarning("Read-back mismatch", fmt.Sprintf("%s/%s was written but reads back differently from what was declared.", s.Unit, s.Ref))
			}
		}
		if ok, _ := out["ok"].(bool); !ok || len(failed) > 0 {
			// Partly applied: keep what landed, tell the user where it stopped.
			_ = finishManifest(m, prior, steps, out, d)
			return &attrError{attr: "manifest_json", summary: "The manifest was only partly applied",
				detail: "The apply stops at the first failure and each finished unit is kept, so fix the manifest and apply again.\n  " + strings.Join(failed, "\n  ")}
		}
		return finishManifest(m, prior, steps, out, d)
	}
	return newResource(&resourceSpec[siteManifestModel]{
		suffix: "site_manifest",
		what:   "site manifest",
		description: "A whole site as one document: the `version: 2` manifest of `PUT /orgs/{orgId}/manifest`, the same one `forumulate plan` and `forumulate apply` use. " +
			"Spaces with their SEO, theme, locales and layout, collections, pages from Markdown, forum categories, sites, domains, webhooks, status monitors, connectors, interface strings and organization settings can all be declared in it.\n\n" +
			"`terraform plan` runs the API's own dry run and prints the manifest plan's steps (create, update, delete, field by field) as warnings, and a manifest the API would refuse fails the plan on `manifest_json`. " +
			"`terraform apply` applies it; the apply is idempotent, additive (nothing undeclared is deleted unless `prune`) and stops at the first failing unit, which is kept. " +
			"A page or setting changed outside Terraform shows as pending changes at the next refresh and is put back by the next apply.\n\n" +
			"Compose the document with Terraform values: `manifest_json = jsonencode({ version = 2, spaces = [...] })`, or `jsonencode(yamldecode(file(\"forumulate.yaml\")))`. " +
			"Destroying the resource forgets the manifest; set `destroy_prunes` to delete what it created (see below).",
		importHelp: "the organization id (the manifest is then adopted: set manifest_json to the document that GET /orgs/{orgId}/manifest?export=true answers)",
		attrs: map[string]schema.Attribute{
			"id": computedID("The organization id."),
			"manifest_json": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The version-2 manifest, as a JSON document. Top-level keys: `version` (must be 2), `spaces`, `sources`, `domains`, `reader_auth`, `readerAuth`, `consent`, `connectors`, `executor`, `webhooks`, `status_monitors`, `brand`, `pages`, `sites`, `translationOverrides`, `orgSettings`, `prune`. " +
					"See the site manifest reference for each block. Secrets in the document (a webhook's, a connector's) are sensitive: mark the whole attribute with `sensitive()` if it carries any.",
				PlanModifiers: []planmodifier.String{jsonEquivalent()},
				Validators:    []validator.String{manifestValidator{}},
			},
			"prune": schema.BoolAttribute{Optional: true, MarkdownDescription: "Delete what the manifest itself created earlier and no longer declares (never anything it did not bring under management; only the kinds the document declares). Sent as the document's `prune`. Do not also put `prune` in `manifest_json`."},
			"destroy_prunes": schema.BoolAttribute{Optional: true,
				MarkdownDescription: "On destroy, apply the manifest with every declared list emptied and `prune: true`, deleting what the manifest created. Off by default: destroying the resource then only stops managing the site."},
			"timeout_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(600), Validators: []validator.Int64{between(30, 3600)},
				MarkdownDescription: "How long to wait for an apply, 30 to 3600 seconds. Defaults to 600. The apply is one request that can take minutes (domains, large page sets)."},
			"pending": schema.StringAttribute{Computed: true, MarkdownDescription: "What the API's dry run would do right now: `none`, or counts like `2 create, 1 update`. Read at every refresh, so drift shows here."},
			"ids": schema.MapAttribute{Computed: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
				MarkdownDescription: "The id of each resource the manifest manages, keyed `<unit>/<ref>` such as `spaces/docs` or `sites/main`, from the last plan or apply."},
			"last_apply_json": schema.StringAttribute{Computed: true, PlanModifiers: keepStr,
				MarkdownDescription: "The last apply's steps as JSON: unit, ref, id, action, status and read-back for each."},
			"manual_json": schema.StringAttribute{Computed: true, PlanModifiers: keepStr,
				MarkdownDescription: "Work only you can do, from the last apply, as JSON: the DNS records to create for a new domain (`kind: dns`, `name`, `type`, `value`) and proxy rules."},
			"secrets_json": schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: keepStr,
				MarkdownDescription: "Secrets the apply minted (a new webhook's signing secret, an executor key), shown by the API once, as sensitive JSON keyed `<unit>/<ref>`. Kept across applies."},
		},
		create: func(ctx context.Context, c *client.Client, m *siteManifestModel, d *diag.Diagnostics) error {
			m.ID = types.StringValue(c.OrgID)
			return run(ctx, c, m, nil, d)
		},
		created: func(m *siteManifestModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *siteManifestModel, d *diag.Diagnostics) error {
			if known(m.ManifestJSON) {
				doc, err := manifestDoc(m)
				if err != nil {
					return nil
				}
				out, err := manifestPut(ctx, manifestClient(c, m), doc, false)
				if err != nil {
					// A manifest the API now refuses is not a reason to lose state.
					d.AddWarning("Could not plan the manifest", err.Error())
					return nil
				}
				steps := parseSteps(out)
				m.Pending = types.StringValue(pendingText(steps))
				m.IDs = mergeIDs(m.IDs, idsOf(steps))
				return nil
			}
			// Imported: there is no document yet; show the org as adoptable.
			m.ID = types.StringValue(c.OrgID)
			m.Pending = types.StringValue("unknown")
			return nil
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *siteManifestModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			return run(ctx, c, plan, prior, d)
		},
		del: func(ctx context.Context, c *client.Client, m *siteManifestModel, d *diag.Diagnostics) error {
			if !m.DestroyPrunes.ValueBool() {
				return nil
			}
			doc, err := manifestDoc(m)
			if err != nil {
				return err
			}
			emptied := map[string]any{"version": json.Number("2"), "prune": true}
			for k, v := range doc {
				switch v.(type) {
				case []any:
					emptied[k] = []any{}
				case map[string]any:
					if k == "translationOverrides" || k == "orgSettings" {
						emptied[k] = map[string]any{}
					}
				}
			}
			out, err := manifestPut(ctx, manifestClient(c, m), emptied, true)
			if err != nil {
				return err
			}
			warnList(d, out)
			return nil
		},
		importID: func(id string) (map[string]string, error) { return map[string]string{"id": id}, nil },
		validate: func(ctx context.Context, cfg *siteManifestModel, d *diag.Diagnostics) {
			if !known(cfg.ManifestJSON) || !known(cfg.Prune) {
				return
			}
			if doc, err := parseObject(cfg.ManifestJSON.ValueString()); err == nil {
				if _, has := doc["prune"]; has {
					d.AddAttributeError(attrPath("prune"), "prune is set twice", "Set prune either as this attribute or inside manifest_json, not both.")
				}
			}
		},
		modifyPlan: func(ctx context.Context, c *client.Client, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
			var plan siteManifestModel
			if resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...); resp.Diagnostics.HasError() {
				return
			}
			// A document built from values not known yet cannot be planned.
			if !known(plan.ManifestJSON) {
				return
			}
			doc, err := manifestDoc(&plan)
			if err != nil {
				return // ValidateConfig reports it
			}
			out, err := manifestPut(ctx, manifestClient(c, &plan), doc, false)
			if err != nil {
				manifestDiag(&resp.Diagnostics, "planning the manifest", err)
				return
			}
			warnList(&resp.Diagnostics, out)
			steps := parseSteps(out)
			text := pendingText(steps)
			var lines []string
			for _, s := range steps {
				if actionable(s) {
					lines = append(lines, "  "+stepLine(s))
				}
			}
			for _, s := range steps {
				if s.Action == "unsupported" {
					resp.Diagnostics.AddWarning("Unsupported manifest unit", fmt.Sprintf("%s/%s cannot be applied on this deployment and is skipped.", s.Unit, s.Ref))
				}
			}
			if man, ok := out["manual"].([]any); ok && len(man) > 0 {
				resp.Diagnostics.AddWarning("Manifest work only you can do", fmt.Sprintf("%d item(s) (DNS records or proxy rules) need doing by hand; see manual_json after the apply.", len(man)))
			}
			if len(lines) > 0 {
				resp.Diagnostics.AddWarning("Manifest plan: "+text, strings.Join(lines, "\n"))
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pending"), types.StringUnknown())...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("ids"), types.MapUnknown(types.StringType))...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("last_apply_json"), types.StringUnknown())...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("manual_json"), types.StringUnknown())...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("secrets_json"), types.StringUnknown())...)
			} else {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pending"), types.StringValue("none"))...)
			}
		},
	})
}

func mergeIDs(a, b types.Map) types.Map {
	vals := map[string]attr.Value{}
	for k, v := range a.Elements() {
		vals[k] = v
	}
	for k, v := range b.Elements() {
		vals[k] = v
	}
	m, _ := types.MapValue(types.StringType, vals)
	return m
}

// finishManifest records an apply's outcome in the model.
func finishManifest(m, prior *siteManifestModel, steps []manifestStep, out Object, d *diag.Diagnostics) error {
	// Only attributes the plan left unknown are filled: a value the plan
	// promised (kept from state) must come back unchanged, or Terraform
	// reports an inconsistent result.
	if m.Pending.IsUnknown() || m.Pending.IsNull() {
		m.Pending = types.StringValue("none")
	}
	if m.IDs.IsUnknown() || m.IDs.IsNull() {
		m.IDs = idsOf(steps)
	}
	if m.LastApplyJSON.IsUnknown() || m.LastApplyJSON.IsNull() {
		if b, err := json.Marshal(applyReport(steps)); err == nil {
			m.LastApplyJSON = types.StringValue(string(b))
		}
	}
	if m.ManualJSON.IsUnknown() || m.ManualJSON.IsNull() {
		manual := out["manual"]
		if manual == nil {
			manual = []any{}
		}
		if b, err := json.Marshal(manual); err == nil {
			m.ManualJSON = types.StringValue(string(b))
		}
	}
	if m.SecretsJSON.IsUnknown() || m.SecretsJSON.IsNull() {
		secrets := map[string]any{}
		if prior != nil && known(prior.SecretsJSON) {
			_ = decodeJSON(prior.SecretsJSON.ValueString(), &secrets)
		}
		for _, s := range steps {
			if len(s.Shown) > 0 {
				secrets[s.Unit+"/"+s.Ref] = s.Shown
			}
		}
		b, _ := json.Marshal(secrets)
		m.SecretsJSON = types.StringValue(string(b))
	}
	return nil
}

func applyReport(steps []manifestStep) []map[string]any {
	rep := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		r := map[string]any{"unit": s.Unit, "ref": s.Ref, "action": s.Action, "status": s.Status}
		if s.ID != "" {
			r["id"] = s.ID
		}
		if s.ReadBack != "" {
			r["readBack"] = s.ReadBack
		}
		if s.Error != "" {
			r["error"] = s.Error
		}
		rep = append(rep, r)
	}
	return rep
}

// manifestValidator checks the document is a version-2 manifest with only the
// keys the API takes, and that its spaces and domains name valid things.
type manifestValidator struct{}

func (manifestValidator) Description(context.Context) string {
	return "a version-2 site manifest as a JSON object"
}
func (v manifestValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (manifestValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	doc, err := parseObject(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid manifest", "The manifest "+err.Error()+".")
		return
	}
	bad := func(msg string) { resp.Diagnostics.AddAttributeError(req.Path, "Invalid manifest", msg) }
	if n, ok := doc["version"].(json.Number); !ok || n.String() != "2" {
		bad("\"version\" must be 2: this resource sends the version-2 manifest (a version-1 document has no domains, webhooks, pages or sites).")
	}
	allowed := map[string]bool{}
	for _, k := range manifestBlocks {
		allowed[k] = true
	}
	var unknown []string
	for k := range doc {
		if !allowed[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		bad(fmt.Sprintf("unrecognised top-level key(s) %s. The API refuses unknown keys by name. Known keys: %s.", strings.Join(unknown, ", "), strings.Join(manifestBlocks, ", ")))
	}
	if _, has := doc["prune"]; has {
		// Allowed in the document, but not together with the attribute (checked in ValidateConfig).
		_ = has
	}
	if spaces, ok := doc["spaces"].([]any); ok {
		for i, s := range spaces {
			o, ok := s.(map[string]any)
			if !ok {
				bad(fmt.Sprintf("spaces[%d] must be an object.", i))
				continue
			}
			slugV, _ := o["slug"].(string)
			if slugV == "" || len(slugV) < 2 || len(slugV) > 48 || !slugRE.MatchString(slugV) {
				bad(fmt.Sprintf("spaces[%d].slug %q must be 2 to 48 characters: lowercase letters, digits and single hyphens.", i, slugV))
			}
			typ, _ := o["type"].(string)
			okType := false
			for _, t := range SpaceTypes {
				okType = okType || t == typ
			}
			if !okType {
				bad(fmt.Sprintf("spaces[%d].type %q is not a space type. One of: %s.", i, typ, strings.Join(SpaceTypes, ", ")))
			}
			if name, _ := o["name"].(string); name == "" || len(name) > 120 {
				bad(fmt.Sprintf("spaces[%d].name must be 1 to 120 characters.", i))
			}
			if vis, ok := o["visibility"].(string); ok && vis != "PUBLIC" && vis != "PRIVATE" {
				bad(fmt.Sprintf("spaces[%d].visibility must be PUBLIC or PRIVATE.", i))
			}
		}
	}
	for _, key := range []string{"domains", "webhooks", "sites", "pages"} {
		if v, ok := doc[key]; ok {
			if _, isArr := v.([]any); !isArr {
				bad(fmt.Sprintf("%q must be a list.", key))
			}
		}
	}
}
