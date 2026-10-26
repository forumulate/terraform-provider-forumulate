package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Collections and forum categories are translated the same way: a map from a
// language code to a name and a description, replaced whole on every write.

var nameDescAttrTypes = map[string]attr.Type{"name": types.StringType, "description": types.StringType}

var langKeyRE = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// nameDescTranslations is the schema attribute for them.
func nameDescTranslations(desc string) schema.MapNestedAttribute {
	return schema.MapNestedAttribute{
		Optional:            true,
		MarkdownDescription: desc + " Keyed by language code (`es`, `pt-br`). Replaced whole when changed; remove the attribute to clear them.",
		Validators:          []validator.Map{mapCheck{keyRE: langKeyRE, keyMsg: "must be a language code like es or pt-br."}},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name":        schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 200)}, MarkdownDescription: "The name in this language (up to 200 characters)."},
			"description": schema.StringAttribute{Optional: true, Validators: []validator.String{length(0, 2000)}, MarkdownDescription: "The description in this language (up to 2000 characters)."},
		}},
	}
}

// translationsBody is the API's `translations` for a plan value: an object of
// {name, description} per language, or null to clear (the plan has none).
func translationsBody(m types.Map) any {
	if !known(m) {
		return nil
	}
	out := map[string]any{}
	for code, e := range m.Elements() {
		o, ok := e.(types.Object)
		if !ok {
			continue
		}
		entry := map[string]any{}
		for k, v := range o.Attributes() {
			if s, ok := v.(types.String); ok && known(s) {
				entry[k] = s.ValueString()
			}
		}
		out[code] = entry
	}
	return out
}

// translationsFromAPI reads them back. Unmanaged (no prior) stays null; when
// managed the API's whole map shows, because a write replaces the whole map.
func translationsFromAPI(prior types.Map, api any) types.Map {
	nullMap := types.MapNull(types.ObjectType{AttrTypes: nameDescAttrTypes})
	if !known(prior) {
		return nullMap
	}
	src, _ := api.(map[string]any)
	vals := map[string]attr.Value{}
	for code, e := range src {
		entry, _ := e.(map[string]any)
		name, desc := getS(entry, "name"), getS(entry, "description")
		// A key the user left unset stays null whatever the API echoes back.
		if p, ok := prior.Elements()[code].(types.Object); ok {
			pa := p.Attributes()
			if v, ok := pa["name"].(types.String); ok && v.IsNull() {
				name = types.StringNull()
			}
			if v, ok := pa["description"].(types.String); ok && v.IsNull() {
				desc = types.StringNull()
			}
		}
		obj, _ := types.ObjectValue(nameDescAttrTypes, map[string]attr.Value{"name": name, "description": desc})
		vals[code] = obj
	}
	out, _ := types.MapValue(types.ObjectType{AttrTypes: nameDescAttrTypes}, vals)
	if len(vals) == 0 && len(prior.Elements()) == 0 {
		return prior
	}
	return out
}
