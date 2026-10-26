package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type createOnlyString struct{ hint string }

func (m createOnlyString) Description(context.Context) string {
	return "Can only be set when the object is created. " + m.hint
}
func (m createOnlyString) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }
func (m createOnlyString) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// On create there is no state; on destroy there is no plan.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if !req.StateValue.Equal(req.PlanValue) {
		resp.Diagnostics.AddAttributeError(req.Path, "This attribute cannot change after creation",
			fmt.Sprintf("The API accepts it only when the object is created, so changing it would be silently ignored. %s", m.hint))
	}
}

// createOnly refuses a change to a string that the API takes only on create,
// rather than accepting the plan and not honouring it.
func createOnly(hint string) planmodifier.String { return createOnlyString{hint: hint} }

type createOnlyBool struct{ hint string }

func (m createOnlyBool) Description(context.Context) string {
	return "Can only be set when the object is created. " + m.hint
}
func (m createOnlyBool) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }
func (m createOnlyBool) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if !req.StateValue.Equal(req.PlanValue) {
		resp.Diagnostics.AddAttributeError(req.Path, "This attribute cannot change after creation",
			fmt.Sprintf("The API accepts it only when the object is created. %s", m.hint))
	}
}
func createOnlyB(hint string) planmodifier.Bool { return createOnlyBool{hint: hint} }

// Shorthands for the modifiers nearly every attribute uses.
var (
	keepStr = []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepInt = []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	keepBln = []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	replStr = []planmodifier.String{stringplanmodifier.RequiresReplace()}
)

// computedID is the `id` attribute every resource has.
func computedID(desc string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, PlanModifiers: keepStr, MarkdownDescription: desc}
}

// reqStr is a required string.
func reqStr(desc string, mods []planmodifier.String, vs ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{Required: true, MarkdownDescription: desc, PlanModifiers: mods, Validators: vs}
}

// optStr is an optional string.
func optStr(desc string, vs ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, MarkdownDescription: desc, Validators: vs}
}

// optComputedStr is an optional string the API fills in when it is left out.
func optComputedStr(desc string, vs ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepStr, MarkdownDescription: desc, Validators: vs}
}

// optComputedBool is an optional bool with a server default.
func optComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepBln, MarkdownDescription: desc}
}

// optComputedInt is an optional integer with a server default.
func optComputedInt(desc string, vs ...validator.Int64) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepInt, MarkdownDescription: desc, Validators: vs}
}

type createOnlyObject struct{ hint string }

func (m createOnlyObject) Description(context.Context) string {
	return "Can only be set when the object is created. " + m.hint
}
func (m createOnlyObject) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (m createOnlyObject) PlanModifyObject(_ context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if !req.StateValue.Equal(req.PlanValue) {
		resp.Diagnostics.AddAttributeError(req.Path, "This attribute cannot change after creation",
			fmt.Sprintf("The API accepts it only when the object is created. %s", m.hint))
	}
}
func createOnlyObj(hint string) planmodifier.Object { return createOnlyObject{hint: hint} }

// warnList reports the API's `warnings` (values it accepted and rewrote) as
// Terraform warnings, so they are seen without failing the apply.
func warnList(d *diag.Diagnostics, o Object) {
	for _, w := range stringsOf(o["warnings"]) {
		d.AddWarning("Forumulate API warning", w)
	}
}

// jsonAttr is a JSON-document attribute (compared by the keys it declares).
func jsonAttr(desc string, keys ...string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: desc + " A JSON document: write it with `jsonencode()`. Drift is detected only for the keys you declare, so settings someone else manages are left alone.",
		Validators:          []validator.String{jsonObject(keys...)},
		PlanModifiers:       []planmodifier.String{jsonEquivalent()},
	}
}
