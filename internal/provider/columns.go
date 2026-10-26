package provider

import (
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
)

// Some objects have dozens of flat fields (a forum category has forty). Rather
// than a line of body-building and a line of reading per field, a resource
// lists them once as fieldMaps and these functions walk the model struct by
// its `tfsdk` tags. The mapping is still one explicit table per resource, and
// a test pins that every row names a real attribute.

// attrFields indexes a model struct's attr.Value fields by tfsdk tag.
func attrFields(model any) map[string]reflect.Value {
	v := reflect.ValueOf(model)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	out := map[string]reflect.Value{}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("tfsdk")
		if tag == "" || tag == "-" {
			continue
		}
		out[tag] = v.Field(i)
	}
	return out
}

func valueOf(f reflect.Value) attr.Value {
	if !f.IsValid() {
		return nil
	}
	if av, ok := f.Interface().(attr.Value); ok {
		return av
	}
	return nil
}

// bodyFromFields is the request body for the table's fields. On update
// (`prior` non-nil) a field that was set and is now null is sent as null, the
// API's way of clearing it; the clearable set limits that to nullable fields.
func bodyFromFields(model any, prior any, table []fieldMap, clearable map[string]bool) Object {
	cur := attrFields(model)
	var old map[string]reflect.Value
	if prior != nil {
		old = attrFields(prior)
	}
	out := Object{}
	for _, f := range table {
		v := valueOf(cur[f.tf])
		if v == nil {
			continue
		}
		if !v.IsNull() && !v.IsUnknown() {
			setPath(out, f.wire, nativeOf(v))
			continue
		}
		if prior != nil && v.IsNull() && clearable[f.tf] {
			if p := valueOf(old[f.tf]); p != nil && !p.IsNull() && !p.IsUnknown() {
				setPath(out, f.wire, nil)
			}
		}
	}
	return out
}

// readFields copies the API object into the model's fields. A field the API
// leaves out or sends as null becomes null. `lenient` fields keep the
// configured spelling when it differs only in case or a leading '#'.
func readFields(model any, table []fieldMap, api Object, lenient map[string]bool) {
	cur := attrFields(model)
	for _, f := range table {
		fv, ok := cur[f.tf]
		if !ok || !fv.CanSet() {
			continue
		}
		prior := valueOf(fv)
		var next attr.Value = nullOf(f)
		if raw, ok := getPath(api, f.wire); ok && raw != nil {
			next = fromWire(f, raw, prior)
		}
		if lenient[f.tf] && f.kind == kStr && prior != nil && !prior.IsNull() && !prior.IsUnknown() && !next.IsNull() {
			a := strings.TrimPrefix(prior.(interface{ ValueString() string }).ValueString(), "#")
			b := strings.TrimPrefix(next.(interface{ ValueString() string }).ValueString(), "#")
			if strings.EqualFold(a, b) {
				continue
			}
		}
		fv.Set(reflect.ValueOf(next))
	}
}
