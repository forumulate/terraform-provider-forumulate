package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Several resources take a whole JSON document as one attribute (a space's
// theme, a site's config, a manifest). The API merges these into what is
// stored, fills defaults, and sends the merged result back, so comparing the
// raw answer with the configuration would show a permanent diff. The helpers
// here compare only what the configuration declares.

func decodeJSON(s string, out any) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after the JSON value")
	}
	return nil
}

// parseObject decodes s as a JSON object.
func parseObject(s string) (map[string]any, error) {
	var v any
	if err := decodeJSON(s, &v); err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a JSON object, like jsonencode({ ... })")
	}
	return m, nil
}

// canonical re-encodes JSON with sorted keys, so two spellings compare equal.
func canonical(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// sameValue is semantic equality of two decoded JSON values (1 == 1.0).
func sameValue(a, b any) bool {
	if canonical(a) == canonical(b) {
		return true
	}
	return reflect.DeepEqual(normalizeNumbers(a), normalizeNumbers(b))
}

func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return x.String()
		}
		return f
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = normalizeNumbers(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeNumbers(e)
		}
		return out
	}
	return v
}

// project restricts `actual` to the shape `declared` mentions: for objects the
// keys declared, recursively; anything else (scalars, arrays) whole. What the
// API added on its own (defaults, siblings someone else manages) is invisible
// to the comparison, while a declared key that changed or vanished shows.
func project(declared, actual any) any {
	d, dok := declared.(map[string]any)
	a, aok := actual.(map[string]any)
	if !dok || !aok {
		return actual
	}
	out := map[string]any{}
	for k, dv := range d {
		av, present := a[k]
		if !present {
			continue
		}
		// A declared null means "make it absent": absence is a match.
		out[k] = project(dv, av)
	}
	return out
}

// dropNulls removes null leaves from a declared document before comparing it:
// `{"a": null}` declares "no a", which the API shows as a missing key.
func dropNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if e == nil {
				continue
			}
			out[k] = dropNulls(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = dropNulls(e)
		}
		return out
	}
	return v
}

// jsonFromAPI is the state value of a JSON attribute: the prior (configured)
// spelling when what the API holds for the declared keys means the same, the
// API's projected document when it does not (that is the drift), and null when
// the API holds nothing and nothing was declared.
func jsonFromAPI(prior types.String, actual any) types.String {
	if known(prior) && prior.ValueString() != "" {
		var declared any
		if decodeJSON(prior.ValueString(), &declared) == nil {
			declared = dropNulls(declared)
			if actual == nil {
				if m, ok := declared.(map[string]any); ok && len(m) == 0 {
					return prior
				}
				return types.StringNull()
			}
			p := project(declared, actual)
			if sameValue(declared, p) {
				return prior
			}
			return types.StringValue(canonical(p))
		}
	}
	// Nothing declared: the attribute is not managed, so whatever the API holds
	// for it (defaults, another tool's settings) is not drift.
	return types.StringNull()
}

// nullRemoved is `planned` plus an explicit null for every key the prior
// document declared and the plan no longer does, at any depth. The API merges
// a PATCH, so dropping a key from the configuration only clears it on the
// server when the request says so.
func nullRemoved(prior, planned any) any {
	p, pok := prior.(map[string]any)
	n, nok := planned.(map[string]any)
	if !pok || !nok {
		return planned
	}
	out := map[string]any{}
	for k, v := range n {
		if pv, had := p[k]; had {
			out[k] = nullRemoved(pv, v)
		} else {
			out[k] = v
		}
	}
	for k := range p {
		if _, still := n[k]; !still {
			out[k] = nil
		}
	}
	return out
}

// mergeBody sets body[key] to the plan's JSON document, with nulls for what
// the prior state declared and the plan dropped. prior may be null (create).
func mergeBody(b Object, key string, prior, plan types.String) {
	mergeBodyDepth(b, key, prior, plan, true)
}

// mergeBodyTop is mergeBody for an endpoint that replaces each top-level key
// whole (a space's `settings`): only the top-level keys the plan dropped get a
// null, because a null nested inside a replaced value would be stored as one.
func mergeBodyTop(b Object, key string, prior, plan types.String) {
	mergeBodyDepth(b, key, prior, plan, false)
}

func mergeBodyDepth(b Object, key string, prior, plan types.String, deep bool) {
	if !known(plan) || plan.ValueString() == "" {
		if known(prior) && prior.ValueString() != "" {
			// The whole attribute was removed: null each top-level key it had.
			var old any
			if decodeJSON(prior.ValueString(), &old) == nil {
				if m, ok := old.(map[string]any); ok && len(m) > 0 {
					b[key] = nullRemoved(old, map[string]any{})
				}
			}
		}
		return
	}
	var cur any
	if err := decodeJSON(plan.ValueString(), &cur); err != nil {
		return
	}
	if known(prior) && prior.ValueString() != "" {
		var old any
		if decodeJSON(prior.ValueString(), &old) == nil {
			if deep {
				cur = nullRemoved(old, cur)
			} else {
				cur = nullRemovedTop(old, cur)
			}
		}
	}
	b[key] = cur
}

// ---- plan modifier + validator for JSON attributes ----

type jsonEquivalentModifier struct{}

func (jsonEquivalentModifier) Description(context.Context) string {
	return "Keeps the stored spelling of a JSON document when the configuration means the same thing."
}
func (m jsonEquivalentModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (jsonEquivalentModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() || req.StateValue.IsUnknown() {
		return
	}
	var a, b any
	if decodeJSON(req.StateValue.ValueString(), &a) != nil || decodeJSON(req.PlanValue.ValueString(), &b) != nil {
		return
	}
	if sameValue(dropNulls(a), dropNulls(b)) && sameValue(a, b) {
		resp.PlanValue = req.StateValue
	}
}

// jsonEquivalent suppresses a diff between two spellings of one document.
func jsonEquivalent() planmodifier.String { return jsonEquivalentModifier{} }

type jsonObjectValidator struct{ keys []string }

func (v jsonObjectValidator) Description(context.Context) string {
	return "must be a JSON object (use jsonencode)"
}
func (v jsonObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v jsonObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	m, err := parseObject(req.ConfigValue.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", "The value "+err.Error()+".")
		return
	}
	if len(v.keys) > 0 {
		allowed := map[string]bool{}
		for _, k := range v.keys {
			allowed[k] = true
		}
		for k := range m {
			if !allowed[k] {
				resp.Diagnostics.AddAttributeError(req.Path, "Unknown key",
					fmt.Sprintf("%q is not a key this attribute takes. The API refuses unknown keys. Known keys: %v.", k, v.keys))
			}
		}
	}
}

// jsonObject validates that a string is a JSON object; with keys, that every
// top-level key is one the API's schema names (the API refuses unknown keys).
func jsonObject(keys ...string) validator.String { return jsonObjectValidator{keys: keys} }

// attrPath is a tiny alias so resource files read cleanly.
func attrPath(name string) path.Path { return path.Root(name) }

// nullRemovedTop is nullRemoved for the top level only.
func nullRemovedTop(prior, planned any) any {
	p, pok := prior.(map[string]any)
	n, nok := planned.(map[string]any)
	if !pok || !nok {
		return planned
	}
	out := map[string]any{}
	for k, v := range n {
		out[k] = v
	}
	for k := range p {
		if _, still := n[k]; !still {
			out[k] = nil
		}
	}
	return out
}
