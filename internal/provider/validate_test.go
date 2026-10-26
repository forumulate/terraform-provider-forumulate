package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// runValidators applies an attribute's validators to a value the way the
// framework does during ValidateResourceConfig and returns the error details.
func runValidators(ctx context.Context, a rschema.Attribute, name string, v attr.Value) []string {
	var out []string
	p := path.Root(name)
	switch x := a.(type) {
	case rschema.StringAttribute:
		for _, val := range x.Validators {
			var r validator.StringResponse
			val.ValidateString(ctx, validator.StringRequest{Path: p, ConfigValue: v.(types.String)}, &r)
			for _, e := range r.Diagnostics.Errors() {
				out = append(out, e.Detail())
			}
		}
	case rschema.Int64Attribute:
		for _, val := range x.Validators {
			var r validator.Int64Response
			val.ValidateInt64(ctx, validator.Int64Request{Path: p, ConfigValue: v.(types.Int64)}, &r)
			for _, e := range r.Diagnostics.Errors() {
				out = append(out, e.Detail())
			}
		}
	case rschema.ListAttribute:
		for _, val := range x.Validators {
			var r validator.ListResponse
			val.ValidateList(ctx, validator.ListRequest{Path: p, ConfigValue: v.(types.List)}, &r)
			for _, e := range r.Diagnostics.Errors() {
				out = append(out, e.Detail())
			}
		}
	case rschema.SetAttribute:
		for _, val := range x.Validators {
			var r validator.SetResponse
			val.ValidateSet(ctx, validator.SetRequest{Path: p, ConfigValue: v.(types.Set)}, &r)
			for _, e := range r.Diagnostics.Errors() {
				out = append(out, e.Detail())
			}
		}
	case rschema.MapAttribute:
		for _, val := range x.Validators {
			var r validator.MapResponse
			val.ValidateMap(ctx, validator.MapRequest{Path: p, ConfigValue: v.(types.Map)}, &r)
			for _, e := range r.Diagnostics.Errors() {
				out = append(out, e.Detail())
			}
		}
	}
	return out
}
