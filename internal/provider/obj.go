package provider

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Object is a decoded API object.
type Object = client.Object

// The functions below turn one key of an API object into a Terraform value.
// A missing key and a JSON null are both Terraform null, so a field the API
// leaves out never reads as a change.

func getS(o Object, k string) types.String {
	switch v := o[k].(type) {
	case string:
		return types.StringValue(v)
	case json.Number:
		return types.StringValue(v.String())
	case bool:
		return types.StringValue(strconv.FormatBool(v))
	}
	return types.StringNull()
}

func getB(o Object, k string) types.Bool {
	if v, ok := o[k].(bool); ok {
		return types.BoolValue(v)
	}
	return types.BoolNull()
}

func getI(o Object, k string) types.Int64 {
	switch v := o[k].(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return types.Int64Value(n)
		}
		if f, err := v.Float64(); err == nil {
			return types.Int64Value(int64(f))
		}
	case float64:
		return types.Int64Value(int64(v))
	}
	return types.Int64Null()
}

func getF(o Object, k string) types.Float64 {
	switch v := o[k].(type) {
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return types.Float64Value(f)
		}
	case float64:
		return types.Float64Value(v)
	}
	return types.Float64Null()
}

// stringsOf is a JSON array of strings as a Go slice (nil when absent).
func stringsOf(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func listOfStrings(ss []string) types.List {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	l, _ := types.ListValue(types.StringType, vals)
	return l
}

func setOfStrings(ss []string) types.Set {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	l, _ := types.SetValue(types.StringType, vals)
	return l
}

// getSL is a string array as a list; null when the API sends null (not set).
func getSL(o Object, k string) types.List {
	if _, ok := o[k].([]any); !ok {
		return types.ListNull(types.StringType)
	}
	return listOfStrings(stringsOf(o[k]))
}

func getSS(o Object, k string) types.Set {
	if _, ok := o[k].([]any); !ok {
		return types.SetNull(types.StringType)
	}
	return setOfStrings(stringsOf(o[k]))
}

// scalarString spells a JSON scalar the way Terraform's map(string) holds it.
func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	}
	return "", false
}

// getSM is an object of scalars as map(string): metadata is declared that way.
func getSM(o Object, k string) types.Map {
	m, ok := o[k].(map[string]any)
	if !ok {
		return types.MapNull(types.StringType)
	}
	vals := map[string]attr.Value{}
	for key, v := range m {
		if s, ok := scalarString(v); ok {
			vals[key] = types.StringValue(s)
		}
	}
	out, _ := types.MapValue(types.StringType, vals)
	return out
}

// ---- values from Terraform into a request body ----

// setStr puts a configured string in the body. When `clear` is set (an update)
// a null value is sent as JSON null, which is how the API clears a field.
func setStr(b Object, k string, v types.String, clear bool) {
	if known(v) {
		b[k] = v.ValueString()
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

func setBool(b Object, k string, v types.Bool, clear bool) {
	if known(v) {
		b[k] = v.ValueBool()
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

func setInt(b Object, k string, v types.Int64, clear bool) {
	if known(v) {
		b[k] = v.ValueInt64()
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

func setFloat(b Object, k string, v types.Float64, clear bool) {
	if known(v) {
		b[k] = v.ValueFloat64()
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

// elementsStrings is the strings of a list or set (nil when null/unknown).
func elementsStrings(v interface {
	IsNull() bool
	IsUnknown() bool
	Elements() []attr.Value
}) []string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := []string{}
	for _, e := range v.Elements() {
		if s, ok := e.(types.String); ok {
			out = append(out, s.ValueString())
		}
	}
	return out
}

func setStrList(b Object, k string, v types.List, clear bool) {
	if known(v) {
		b[k] = elementsStrings(v)
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

func setStrSet(b Object, k string, v types.Set, clear bool) {
	if known(v) {
		b[k] = elementsStrings(v)
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}

// setStrMap sends map(string) as an object. Values that read as a boolean or
// a number keep that type, because metadata and org settings accept all three.
func setStrMap(b Object, k string, v types.Map, clear bool) {
	if known(v) {
		m := map[string]any{}
		for key, e := range v.Elements() {
			if s, ok := e.(types.String); ok {
				m[key] = s.ValueString()
			}
		}
		b[k] = m
	} else if clear && v.IsNull() {
		b[k] = map[string]any{}
	}
}

// setJSON sends a JSON-encoded attribute as the structure it holds.
func setJSON(b Object, k string, v types.String, clear bool) {
	if known(v) && strings.TrimSpace(v.ValueString()) != "" {
		var x any
		if err := decodeJSON(v.ValueString(), &x); err == nil {
			b[k] = x
		}
	} else if clear && v.IsNull() {
		b[k] = nil
	}
}
