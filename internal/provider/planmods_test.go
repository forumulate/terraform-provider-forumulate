package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func rawObj(null bool) tfsdk.Plan {
	v := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	if null {
		v = tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, nil)
	}
	return tfsdk.Plan{Raw: v}
}

func TestCreateOnlyRefusesAChangeAfterCreation(t *testing.T) {
	run := func(state types.String, plan types.String, hasState bool) bool {
		st := tfsdk.State{Raw: rawObj(!hasState).Raw}
		req := planmodifier.StringRequest{Path: path.Root("preset"), State: st, Plan: rawObj(false), PlanValue: plan, StateValue: state}
		var resp planmodifier.StringResponse
		createOnly("Use forumulate_space_settings.").PlanModifyString(context.Background(), req, &resp)
		return resp.Diagnostics.HasError()
	}
	if run(types.StringNull(), S("wide"), false) {
		t.Error("setting it on create is fine")
	}
	if run(S("wide"), S("wide"), true) {
		t.Error("no change is fine")
	}
	if !run(S("wide"), S("narrow"), true) {
		t.Error("a change after creation must be refused, not silently ignored")
	}
	if !run(S("wide"), types.StringNull(), true) {
		t.Error("removing it after creation must be refused too")
	}
	if run(S("wide"), types.StringUnknown(), true) {
		t.Error("an unknown value cannot be judged yet")
	}
}

func TestCreateOnlyBoolAndObject(t *testing.T) {
	st := tfsdk.State{Raw: rawObj(false).Raw}
	var br planmodifier.BoolResponse
	createOnlyB("").PlanModifyBool(context.Background(), planmodifier.BoolRequest{Path: path.Root("seed"), State: st, Plan: rawObj(false), PlanValue: types.BoolValue(true), StateValue: types.BoolValue(false)}, &br)
	if !br.Diagnostics.HasError() {
		t.Error("seed change must be refused")
	}
	var or planmodifier.ObjectResponse
	a := types.ObjectNull(shapeTypes)
	b := types.ObjectValueMust(shapeTypes, map[string]attr.Value{"index": S("list"), "order": types.StringNull(), "nav": types.StringNull(), "who_creates": types.StringNull()})
	createOnlyObj("").PlanModifyObject(context.Background(), planmodifier.ObjectRequest{Path: path.Root("shape"), State: st, Plan: rawObj(false), PlanValue: b, StateValue: a}, &or)
	if !or.Diagnostics.HasError() {
		t.Error("shape change must be refused")
	}
}

func TestJSONEquivalentKeepsTheStoredSpelling(t *testing.T) {
	m := jsonEquivalent()
	run := func(state, plan string) types.String {
		req := planmodifier.StringRequest{PlanValue: S(plan), StateValue: S(state)}
		resp := planmodifier.StringResponse{PlanValue: S(plan)}
		m.PlanModifyString(context.Background(), req, &resp)
		return resp.PlanValue
	}
	if got := run(`{"a":1,"b":2}`, `{ "b": 2, "a": 1.0 }`); got.ValueString() != `{"a":1,"b":2}` {
		t.Errorf("equivalent spelling must keep the state: %s", got.ValueString())
	}
	if got := run(`{"a":1}`, `{"a":2}`); got.ValueString() != `{"a":2}` {
		t.Errorf("a real change must stay: %s", got.ValueString())
	}
	if got := run(`{"a":1}`, `{"a":1,"b":null}`); got.ValueString() != `{"a":1,"b":null}` {
		t.Errorf("a declared null is a change (it clears b): %s", got.ValueString())
	}
}
