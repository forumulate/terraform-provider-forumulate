package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A nested object attribute (a collection's nav, a category's look) maps field
// for field onto an object in the API. fieldMap says how, so one pair of
// functions reads and writes every such attribute.

type fieldKind int

const (
	kStr fieldKind = iota
	kBool
	kInt
	kFloat
	kStrSet
	kStrList
)

// fieldMap ties a Terraform attribute to an API key; wire may be dotted
// ("audience.principals") for a key one object deeper.
type fieldMap struct {
	tf   string
	wire string
	kind fieldKind
}

func (f fieldMap) attrType() attr.Type {
	switch f.kind {
	case kBool:
		return types.BoolType
	case kInt:
		return types.Int64Type
	case kFloat:
		return types.Float64Type
	case kStrSet:
		return types.SetType{ElemType: types.StringType}
	case kStrList:
		return types.ListType{ElemType: types.StringType}
	}
	return types.StringType
}

func attrTypesOf(fields []fieldMap) map[string]attr.Type {
	out := map[string]attr.Type{}
	for _, f := range fields {
		out[f.tf] = f.attrType()
	}
	return out
}

func getPath(o Object, wire string) (any, bool) {
	var cur any = o
	for _, seg := range strings.Split(wire, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setPath(o Object, wire string, v any) {
	segs := strings.Split(wire, ".")
	cur := o
	for _, seg := range segs[:len(segs)-1] {
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
	cur[segs[len(segs)-1]] = v
}

// objectBody is the API object for a Terraform object attribute: its set
// fields as their values. With `clearFrom` (the prior object), a field the
// prior had and the plan dropped is sent as null so the API removes it.
func objectBody(obj types.Object, fields []fieldMap, clearFrom *types.Object) Object {
	out := Object{}
	var prior map[string]attr.Value
	if clearFrom != nil && known(*clearFrom) {
		prior = clearFrom.Attributes()
	}
	var cur map[string]attr.Value
	if known(obj) {
		cur = obj.Attributes()
	}
	for _, f := range fields {
		v := cur[f.tf]
		if v != nil && !v.IsNull() && !v.IsUnknown() {
			setPath(out, f.wire, nativeOf(v))
			continue
		}
		if p := prior[f.tf]; p != nil && !p.IsNull() {
			setPath(out, f.wire, nil)
		}
	}
	return out
}

func nativeOf(v attr.Value) any {
	switch x := v.(type) {
	case types.String:
		return x.ValueString()
	case types.Bool:
		return x.ValueBool()
	case types.Int64:
		return x.ValueInt64()
	case types.Float64:
		return x.ValueFloat64()
	case types.Set:
		return elementsStrings(x)
	case types.List:
		return elementsStrings(x)
	}
	return nil
}

// objectFromAPI is the state value of a nested object. Only the fields the
// configuration declared (the prior object's non-null attributes) are read
// back, so a default the API adds is not drift. When `whole` is set the API
// owns the object entirely (a computed attribute) and everything shows.
func objectFromAPI(prior types.Object, fields []fieldMap, api Object, whole bool) types.Object {
	at := attrTypesOf(fields)
	if !whole && !known(prior) {
		return types.ObjectNull(at)
	}
	declared := map[string]attr.Value{}
	if known(prior) {
		declared = prior.Attributes()
	}
	vals := map[string]attr.Value{}
	for _, f := range fields {
		var v attr.Value = nullOf(f)
		if p := declared[f.tf]; whole || (p != nil && !p.IsNull()) {
			if raw, ok := getPath(api, f.wire); ok && raw != nil {
				v = fromWire(f, raw, p)
			}
		}
		vals[f.tf] = v
	}
	obj, _ := types.ObjectValue(at, vals)
	return obj
}

func nullOf(f fieldMap) attr.Value {
	switch f.kind {
	case kBool:
		return types.BoolNull()
	case kInt:
		return types.Int64Null()
	case kFloat:
		return types.Float64Null()
	case kStrSet:
		return types.SetNull(types.StringType)
	case kStrList:
		return types.ListNull(types.StringType)
	}
	return types.StringNull()
}

func fromWire(f fieldMap, raw any, prior attr.Value) attr.Value {
	o := Object{"v": raw}
	switch f.kind {
	case kBool:
		return getB(o, "v")
	case kInt:
		return getI(o, "v")
	case kFloat:
		return getF(o, "v")
	case kStrSet:
		return getSS(o, "v")
	case kStrList:
		return getSL(o, "v")
	}
	return getS(o, "v")
}
