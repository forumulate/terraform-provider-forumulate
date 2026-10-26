package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// resourceSpec is everything one resource says about itself; genericResource
// turns it into a resource.Resource. The framework's CRUD plumbing (read the
// plan, check the token, map an API error onto the attribute it names, remove
// a 404 from state, write the result back) is identical for every resource, so
// it lives here once and a resource file holds only what is particular to it.
type resourceSpec[M any] struct {
	// suffix is the type name after `forumulate_`.
	suffix string
	// description is the Markdown shown in the registry docs.
	description string
	attrs       map[string]schema.Attribute
	blocks      map[string]schema.Block
	// what names the thing in error summaries: "space", "forum category".
	what string
	// paramAlias maps an API body key to the attribute that carries it, for the
	// few that do not follow the snake_case(+_json) rule.
	paramAlias map[string]string

	create func(ctx context.Context, c *client.Client, plan *M, d *diag.Diagnostics) error
	// read refreshes state from the API. A client.IsNotFound error removes the
	// resource from state (it was deleted outside Terraform).
	read   func(ctx context.Context, c *client.Client, state *M, d *diag.Diagnostics) error
	update func(ctx context.Context, c *client.Client, plan, prior *M, d *diag.Diagnostics) error
	del    func(ctx context.Context, c *client.Client, state *M, d *diag.Diagnostics) error

	// importID turns `terraform import` ids into the attributes that identify
	// the resource; read fills the rest.
	importID func(id string) (map[string]string, error)
	// importHelp is the id format, for the error and the docs.
	importHelp string

	// validate is cross-attribute validation that needs the whole config.
	validate func(ctx context.Context, cfg *M, d *diag.Diagnostics)
	// modifyPlan may annotate the plan (the manifest shows its dry run here).
	modifyPlan func(ctx context.Context, c *client.Client, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse)
	// created reports that a failed create already made the remote object, so
	// its id is saved and Terraform marks the resource tainted instead of
	// forgetting something that exists.
	created func(m *M) bool
	// noToken lets a resource work without a token (none today).
	noToken bool
}

// genericResource implements resource.Resource for a spec.
type genericResource[M any] struct {
	spec *resourceSpec[M]
	c    *client.Client
}

var (
	_ resource.Resource = &genericResource[struct{}]{}
)

func newResource[M any](spec *resourceSpec[M]) resource.Resource {
	return &genericResource[M]{spec: spec}
}

func (r *genericResource[M]) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.suffix
}

func (r *genericResource[M]) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.spec.description,
		Attributes:          r.spec.attrs,
		Blocks:              r.spec.blocks,
	}
}

func (r *genericResource[M]) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.c = c
	}
}

// attrNames are the attribute names API errors may be pinned to.
func (r *genericResource[M]) attrNames() []string {
	names := make([]string, 0, len(r.spec.attrs))
	for k := range r.spec.attrs {
		names = append(names, k)
	}
	for k := range r.spec.blocks {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (r *genericResource[M]) fail(d *diag.Diagnostics, verb string, err error) {
	apiDiagFor(d, verb+" "+r.spec.what, err, r.attrNames(), r.spec.paramAlias)
}

func (r *genericResource[M]) ready(d *diag.Diagnostics) bool {
	if r.spec.noToken {
		return r.c != nil
	}
	return !needsToken(r.c, d)
}

func (r *genericResource[M]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.ready(&resp.Diagnostics) {
		return
	}
	if err := r.spec.create(ctx, r.c, &plan, &resp.Diagnostics); err != nil {
		r.fail(&resp.Diagnostics, "creating", err)
		if r.spec.created != nil && r.spec.created(&plan) {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		}
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *genericResource[M]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.ready(&resp.Diagnostics) {
		return
	}
	if err := r.spec.read(ctx, r.c, &state, &resp.Diagnostics); err != nil {
		if client.IsNotFound(err) {
			// Deleted outside Terraform: the next plan proposes creating it.
			resp.State.RemoveResource(ctx)
			return
		}
		r.fail(&resp.Diagnostics, "reading", err)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *genericResource[M]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior M
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() || !r.ready(&resp.Diagnostics) {
		return
	}
	if err := r.spec.update(ctx, r.c, &plan, &prior, &resp.Diagnostics); err != nil {
		r.fail(&resp.Diagnostics, "updating", err)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *genericResource[M]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state M
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.ready(&resp.Diagnostics) {
		return
	}
	if r.spec.del == nil {
		return
	}
	if err := r.spec.del(ctx, r.c, &state, &resp.Diagnostics); err != nil && !client.IsNotFound(err) {
		r.fail(&resp.Diagnostics, "deleting", err)
	}
}

func (r *genericResource[M]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.spec.importID == nil {
		resp.Diagnostics.AddError("Import is not supported", "forumulate_"+r.spec.suffix+" cannot be imported.")
		return
	}
	attrs, err := r.spec.importID(req.ID)
	if err != nil {
		help := ""
		if r.spec.importHelp != "" {
			help = " Use " + r.spec.importHelp + "."
		}
		resp.Diagnostics.AddError("Invalid import id", err.Error()+help)
		return
	}
	for k, v := range attrs {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k), v)...)
	}
}

// ValidateConfig runs the resource's cross-attribute checks.
func (r *genericResource[M]) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.spec.validate == nil {
		return
	}
	var cfg M
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.spec.validate(ctx, &cfg, &resp.Diagnostics)
}

// ModifyPlan lets a resource annotate its plan.
func (r *genericResource[M]) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.spec.modifyPlan == nil || req.Plan.Raw.IsNull() {
		return
	}
	if r.c == nil || r.c.Token == "" {
		return
	}
	r.spec.modifyPlan(ctx, r.c, req, resp)
}

// splitID cuts an import id into exactly n parts on "/".
func splitID(id string, n int) ([]string, error) {
	parts := strings.SplitN(id, "/", n)
	if len(parts) != n {
		return nil, fmt.Errorf("%q does not have %d parts", id, n)
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("%q has an empty part", id)
		}
	}
	return parts, nil
}

// snake turns an API key (camelCase) into a Terraform attribute name.
func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
