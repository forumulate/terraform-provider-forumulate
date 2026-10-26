package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeAPI is an httptest server standing in for the Forumulate API. Tests
// register handlers by "METHOD /path/{wildcards}" (Go 1.22 mux patterns) and
// read back every request the provider made.
type fakeAPI struct {
	t     *testing.T
	srv   *httptest.Server
	mux   *http.ServeMux
	mu    sync.Mutex
	calls []call
}

type call struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
	Header http.Header
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, mux: http.NewServeMux()}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			dec.UseNumber()
			_ = dec.Decode(&body)
		}
		f.mu.Lock()
		f.calls = append(f.calls, call{r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header.Clone()})
		f.mu.Unlock()
		// Handlers read the body from the recorded call, not from r.Body.
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		f.mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// on registers a handler; it answers JSON with status.
func (f *fakeAPI) on(pattern string, status int, resp func(r *http.Request, body map[string]any) any) {
	f.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
		out := resp(r, body)
		w.Header().Set("Content-Type", "application/json")
		if e, ok := out.(apiFail); ok {
			w.WriteHeader(e.status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": e.env, "requestId": "req_test"})
			return
		}
		w.WriteHeader(status)
		if out != nil {
			_ = json.NewEncoder(w).Encode(out)
		}
	})
}

// onText registers a plain-text handler.
func (f *fakeAPI) onText(pattern string, text func() string) {
	f.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte(text()))
	})
}

type apiFail struct {
	status int
	env    map[string]any
}

// fail is the API's error envelope.
func fail(status int, code, message, param string) apiFail {
	env := map[string]any{"code": code, "message": message}
	if param != "" {
		env["param"] = param
	}
	return apiFail{status, env}
}

func (f *fakeAPI) client() *client.Client {
	c := client.New(f.srv.URL, "fp_test", "org1")
	c.Sleep = func(time.Duration) {}
	return c
}

func (f *fakeAPI) last() call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// find returns the calls matching "METHOD path".
func (f *fakeAPI) find(method, path string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.Method == method && c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeAPI) reset() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}

// driver runs one resource's CRUD through the framework's own interfaces: the
// plan, state and config are built from the resource's schema, exactly as the
// plugin server builds them, so what passes here passes the real protocol.
type driver[M any] struct {
	t   *testing.T
	r   resource.Resource
	sch resourceSchema
	ctx context.Context
}

type resourceSchema = rschema.Schema

func newDriver[M any](t *testing.T, f *fakeAPI, ctor func() resource.Resource) *driver[M] {
	t.Helper()
	r := ctor()
	ctx := context.Background()
	var sresp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sresp)
	if d := sresp.Schema.ValidateImplementation(ctx); d.HasError() {
		t.Fatalf("schema invalid: %v", d)
	}
	r.(resource.ResourceWithConfigure).Configure(ctx, resource.ConfigureRequest{ProviderData: f.client()}, &resource.ConfigureResponse{})
	return &driver[M]{t: t, r: r, sch: sresp.Schema, ctx: ctx}
}

func (d *driver[M]) nullState() tfsdk.State {
	return tfsdk.State{Schema: d.sch, Raw: tftypes.NewValue(d.sch.Type().TerraformType(d.ctx), nil)}
}

func (d *driver[M]) plan(m M) tfsdk.Plan {
	p := tfsdk.Plan{Schema: d.sch, Raw: tftypes.NewValue(d.sch.Type().TerraformType(d.ctx), nil)}
	if diags := p.Set(d.ctx, &m); diags.HasError() {
		d.t.Fatalf("plan.Set: %v", diags)
	}
	return p
}

func (d *driver[M]) state(m M) tfsdk.State {
	s := d.nullState()
	if diags := s.Set(d.ctx, &m); diags.HasError() {
		d.t.Fatalf("state.Set: %v", diags)
	}
	return s
}

func (d *driver[M]) get(s tfsdk.State) M {
	var m M
	if diags := s.Get(d.ctx, &m); diags.HasError() {
		d.t.Fatalf("state.Get: %v", diags)
	}
	return m
}

// create runs Create and returns the state the framework would store.
func (d *driver[M]) create(plan M) (M, diag.Diagnostics) {
	resp := resource.CreateResponse{State: d.nullState()}
	d.r.Create(d.ctx, resource.CreateRequest{Plan: d.plan(plan)}, &resp)
	// A failed create that already made the remote object keeps its state
	// (the framework taints it); one that made nothing leaves none.
	if resp.State.Raw.IsNull() {
		var zero M
		return zero, resp.Diagnostics
	}
	return d.get(resp.State), resp.Diagnostics
}

// read runs Read; gone is true when the resource removed itself from state.
func (d *driver[M]) read(prior M) (m M, gone bool, diags diag.Diagnostics) {
	resp := resource.ReadResponse{State: d.state(prior)}
	d.r.Read(d.ctx, resource.ReadRequest{State: d.state(prior)}, &resp)
	if resp.Diagnostics.HasError() {
		return prior, false, resp.Diagnostics
	}
	if resp.State.Raw.IsNull() {
		return prior, true, resp.Diagnostics
	}
	return d.get(resp.State), false, resp.Diagnostics
}

func (d *driver[M]) update(plan, prior M) (M, diag.Diagnostics) {
	resp := resource.UpdateResponse{State: d.state(prior)}
	d.r.Update(d.ctx, resource.UpdateRequest{Plan: d.plan(plan), State: d.state(prior)}, &resp)
	if resp.Diagnostics.HasError() {
		return prior, resp.Diagnostics
	}
	return d.get(resp.State), resp.Diagnostics
}

func (d *driver[M]) delete(prior M) diag.Diagnostics {
	resp := resource.DeleteResponse{State: d.state(prior)}
	d.r.Delete(d.ctx, resource.DeleteRequest{State: d.state(prior)}, &resp)
	return resp.Diagnostics
}

// importState runs ImportState then Read, as `terraform import` does.
func (d *driver[M]) importState(id string) (M, bool, diag.Diagnostics) {
	resp := resource.ImportStateResponse{State: d.nullState()}
	d.r.(resource.ResourceWithImportState).ImportState(d.ctx, resource.ImportStateRequest{ID: id}, &resp)
	if resp.Diagnostics.HasError() {
		var zero M
		return zero, false, resp.Diagnostics
	}
	rr := resource.ReadResponse{State: resp.State}
	d.r.Read(d.ctx, resource.ReadRequest{State: resp.State}, &rr)
	if rr.Diagnostics.HasError() || rr.State.Raw.IsNull() {
		var zero M
		return zero, rr.State.Raw.IsNull(), rr.Diagnostics
	}
	return d.get(rr.State), false, rr.Diagnostics
}

// validate runs the resource's ValidateConfig over a config built from m.
func (d *driver[M]) validate(m M) diag.Diagnostics {
	cfg := tfsdk.Config{Schema: d.sch, Raw: d.plan(m).Raw}
	resp := resource.ValidateConfigResponse{}
	if v, ok := d.r.(resource.ResourceWithValidateConfig); ok {
		v.ValidateConfig(d.ctx, resource.ValidateConfigRequest{Config: cfg}, &resp)
	}
	return resp.Diagnostics
}

// attrErrors runs the attribute-level validators the framework would run for
// one top-level attribute of the resource and returns the error details.
func (d *driver[M]) attrErrors(name string, v attr.Value) []string {
	d.t.Helper()
	a, ok := d.sch.Attributes[name]
	if !ok {
		d.t.Fatalf("no attribute %q", name)
	}
	return runValidators(d.ctx, a, name, v)
}

func details(d diag.Diagnostics) string {
	var b strings.Builder
	for _, e := range d {
		b.WriteString(e.Summary() + ": " + e.Detail() + "\n")
	}
	return b.String()
}

func mustNoErr(t *testing.T, d diag.Diagnostics) {
	t.Helper()
	if d.HasError() {
		t.Fatalf("unexpected diagnostics:\n%s", details(d))
	}
}

func mustErr(t *testing.T, d diag.Diagnostics, contains string) {
	t.Helper()
	if !d.HasError() {
		t.Fatalf("expected an error containing %q, got none", contains)
	}
	if !strings.Contains(details(d), contains) {
		t.Fatalf("error %q does not contain %q", details(d), contains)
	}
}

func S(s string) types.String { return types.StringValue(s) }

var _ = datasource.ReadRequest{}
var _ validator.String
