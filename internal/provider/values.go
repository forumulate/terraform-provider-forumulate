package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// known reports a value the plan actually carries (not null, not unknown).
func known(v interface {
	IsNull() bool
	IsUnknown() bool
}) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// setNullable puts an optional, clearable field in a body: its value, or an
// explicit null on update so removing it from configuration clears it on the
// server (the API treats null as "clear" for these fields).
func setNullable(body map[string]any, key string, v types.String, update bool) {
	if known(v) {
		body[key] = v.ValueString()
	} else if update && v.IsNull() {
		body[key] = nil
	}
}

func setNullableInt(body map[string]any, key string, v types.Int64, update bool) {
	if known(v) {
		body[key] = v.ValueInt64()
	} else if update && v.IsNull() {
		body[key] = nil
	}
}

func setNullableFloat(body map[string]any, key string, v types.Float64, update bool) {
	if known(v) {
		body[key] = v.ValueFloat64()
	} else if update && v.IsNull() {
		body[key] = nil
	}
}

// strOrNull is a *string from the API as a Terraform string.
func strOrNull(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func intOrNull(n *int64) types.Int64 {
	if n == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*n)
}

func floatOrNull(n *float64) types.Float64 {
	if n == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*n)
}

// sameJSON is semantic JSON equality, so `jsonencode()` key order or spacing in
// configuration never reads as drift against what the API sends back.
func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

// probeState is the state's probe_json given what the API holds: the
// configured spelling when it means the same thing, else the API's.
func probeState(prior types.String, api json.RawMessage) types.String {
	if len(api) == 0 || string(api) == "null" {
		return types.StringNull()
	}
	if known(prior) && sameJSON([]byte(prior.ValueString()), api) {
		return prior
	}
	return types.StringValue(string(api))
}

// attrError is an error a resource raises about one attribute it has already
// found wrong (before or after calling the API); apiDiagFor pins it there.
type attrError struct {
	attr, summary, detail string
}

func (e *attrError) Error() string { return e.detail }

// apiDiag turns a client error into a diagnostic that keeps the API's words.
func apiDiag(d *diag.Diagnostics, what string, err error) {
	apiDiagFor(d, what, err, nil, nil)
}

// apiDiagFor is apiDiag that also pins a validation error to the attribute it
// names. The API's error envelope carries `param` (the first field at fault)
// and `details` (every field, by dotted path); a path like
// `config.pageMounts.0.path` becomes the attribute `page_mounts` or
// `config_json`, so Terraform underlines the line in the configuration that is
// wrong instead of printing a bare 422.
func apiDiagFor(d *diag.Diagnostics, what string, err error, attrs []string, alias map[string]string) {
	summary := "Forumulate API: " + what
	var ae *attrError
	if errors.As(err, &ae) {
		d.AddAttributeError(path.Root(ae.attr), ae.summary, ae.detail)
		return
	}
	e, ok := client.AsAPIError(err)
	if !ok {
		d.AddError(summary, err.Error())
		return
	}
	detail := e.Message
	if e.Code != "" {
		detail = e.Code + ": " + e.Message
	}
	if e.Hint != "" {
		detail += "\n\nHint: " + e.Hint
	}
	if e.Docs != "" {
		detail += "\n\nSee: " + e.Docs
	}
	if e.RequestID != "" {
		detail += "\n\nRequest id: " + e.RequestID
	}
	candidates := []string{}
	if e.Param != "" {
		candidates = append(candidates, e.Param)
	}
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	candidates = append(candidates, keys...)
	for _, c := range candidates {
		if name := attrFor(c, attrs, alias); name != "" {
			if msgs := e.Details[c]; len(msgs) > 0 {
				detail += "\n\n" + c + ": " + strings.Join(msgs, "; ")
			}
			d.AddAttributeError(path.Root(name), summary, detail)
			return
		}
	}
	d.AddError(summary, detail)
}

// attrFor maps an API field path to one of the resource's attributes: the
// alias table first, then snake_case of the first segment, then that with
// `_json` (a document attribute carries the whole object).
func attrFor(param string, attrs []string, alias map[string]string) string {
	param = strings.TrimPrefix(param, "body")
	param = strings.TrimPrefix(param, ".")
	if param == "" {
		return ""
	}
	segs := strings.Split(param, ".")
	first := segs[0]
	if i := strings.IndexByte(first, '['); i > 0 {
		first = first[:i]
	}
	have := map[string]bool{}
	for _, a := range attrs {
		have[a] = true
	}
	// A site's `config.pageMounts.0.path` is the page_mounts attribute, not
	// the config document: the key under `config` names the attribute when
	// it has one of its own.
	if first == "config" && len(segs) > 1 && have[snake(segs[1])] {
		return snake(segs[1])
	}
	for _, c := range []string{alias[first], snake(first), snake(first) + "_json"} {
		if c != "" && have[c] {
			return c
		}
	}
	return ""
}

// needsToken is the error every managed resource gives when the provider has
// no token: the API would answer 401, which says less.
func needsToken(c *client.Client, d *diag.Diagnostics) bool {
	if c == nil || c.Token == "" {
		d.AddError("Missing Forumulate token",
			"Managing Forumulate objects needs a personal access token: set token in the provider block or FORUMULATE_TOKEN.")
		return true
	}
	return false
}
