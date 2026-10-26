package provider

import (
	stdjson "encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTranslationOverrideCRUD(t *testing.T) {
	f := newFakeAPI(t)
	store := map[string]string{}
	f.on("PUT /orgs/org1/translation-overrides", 200, func(r *http.Request, b map[string]any) any {
		store[b["locale"].(string)+"/"+b["key"].(string)] = b["value"].(string)
		return map[string]any{"override": map[string]any{"key": b["key"], "locale": b["locale"], "value": b["value"]}}
	})
	f.on("GET /orgs/org1/translation-overrides", 200, func(r *http.Request, b map[string]any) any {
		rows := []any{}
		for k, v := range store {
			loc, key, _ := splitKey(k)
			if loc == r.URL.Query().Get("locale") {
				rows = append(rows, map[string]any{"locale": loc, "key": key, "value": v})
			}
		}
		return map[string]any{"data": rows}
	})
	f.on("DELETE /orgs/org1/translation-overrides", 200, func(r *http.Request, b map[string]any) any {
		delete(store, r.URL.Query().Get("locale")+"/"+r.URL.Query().Get("key"))
		return map[string]any{"ok": true}
	})
	d := newDriver[translationOverrideModel](t, f, NewTranslationOverrideResource)
	state, diags := d.create(translationOverrideModel{ID: types.StringUnknown(), Locale: S("es"), Key: S("reader.sign_in"), Value: S("Entrar")})
	mustNoErr(t, diags)
	if state.ID.ValueString() != "es/reader.sign_in" {
		t.Fatalf("id = %q", state.ID.ValueString())
	}
	store["es/reader.sign_in"] = "Acceder"
	read, _, diags := d.read(state)
	mustNoErr(t, diags)
	if read.Value.ValueString() != "Acceder" {
		t.Fatalf("drift not detected: %q", read.Value.ValueString())
	}
	up := state
	up.Value = S("Iniciar sesión")
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	if store["es/reader.sign_in"] != "Iniciar sesión" {
		t.Fatal("update did not write")
	}
	mustNoErr(t, d.delete(state))
	if len(store) != 0 {
		t.Fatalf("delete must remove the override: %v", store)
	}
	_, gone, diags := d.read(state)
	mustNoErr(t, diags)
	if !gone {
		t.Fatal("a removed override must leave state")
	}
	imp, _, diags := d.importState("es/a.b")
	mustNoErr(t, diags)
	_ = imp
	if errs := d.attrErrors("locale", S("zz")); len(errs) == 0 {
		t.Error("locale enum")
	}
	if errs := d.attrErrors("value", S(string(make([]byte, 4001)))); len(errs) == 0 {
		t.Error("value max 4000")
	}
}

func splitKey(k string) (string, string, bool) {
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			return k[:i], k[i+1:], true
		}
	}
	return k, "", false
}

func TestOrgSettingTypesBoundsAndReset(t *testing.T) {
	f := newFakeAPI(t)
	rows := map[string]map[string]any{
		"search.typo_tolerance": {"key": "search.typo_tolerance", "type": "boolean", "default": true, "value": true, "isDefault": true},
		"trash.retention_days":  {"key": "trash.retention_days", "type": "integer", "default": num(30), "value": num(30), "isDefault": true, "min": num(1), "max": num(365)},
		"mail.digest":           {"key": "mail.digest", "type": "enum", "options": []any{"daily", "weekly"}, "default": "daily", "value": "daily", "isDefault": true},
	}
	f.on("GET /orgs/org1/settings/{key}", 200, func(r *http.Request, b map[string]any) any {
		row, ok := rows[r.PathValue("key")]
		if !ok {
			return fail(404, "not_found", "No such setting.", "key")
		}
		return map[string]any{"setting": row}
	})
	f.on("PUT /orgs/org1/settings/{key}", 200, func(r *http.Request, b map[string]any) any {
		row := rows[r.PathValue("key")]
		row["value"], row["isDefault"] = b["value"], false
		return map[string]any{"setting": row}
	})
	f.on("DELETE /orgs/org1/settings/{key}", 200, func(r *http.Request, b map[string]any) any {
		row := rows[r.PathValue("key")]
		row["value"], row["isDefault"] = row["default"], true
		return map[string]any{"setting": row}
	})
	d := newDriver[orgSettingModel](t, f, NewOrgSettingResource)
	mk := func(key, value string) orgSettingModel {
		return orgSettingModel{ID: types.StringUnknown(), Key: S(key), Value: S(value), Type: types.StringUnknown(), IsDefault: types.BoolUnknown(), DefaultValue: types.StringUnknown()}
	}

	st, diags := d.create(mk("search.typo_tolerance", "false"))
	mustNoErr(t, diags)
	if f.last().Body["value"] != false {
		t.Fatalf("a boolean must be sent as a boolean: %v", f.last().Body)
	}
	if st.Type.ValueString() != "boolean" || st.IsDefault.ValueBool() || st.DefaultValue.ValueString() != "true" {
		t.Fatalf("state = %+v", st)
	}
	if st.Value.ValueString() != "false" {
		t.Fatalf("value = %q", st.Value.ValueString())
	}

	n, diags := d.create(mk("trash.retention_days", "90"))
	mustNoErr(t, diags)
	if v := f.last().Body["value"]; v != "90" && v.(interface{ String() string }).String() != "90" {
		t.Fatalf("integer body = %#v", v)
	}
	// "090" and "90" are one value: the configured spelling is kept on read.
	n.Value = S("090")
	rd, _, diags := d.read(n)
	mustNoErr(t, diags)
	if rd.Value.ValueString() != "090" {
		t.Fatalf("spelling not kept: %q", rd.Value.ValueString())
	}

	_, diags = d.create(mk("trash.retention_days", "9000"))
	mustErr(t, diags, "at most 365")
	if diags.Errors()[0].Summary() != "Invalid value for trash.retention_days" {
		t.Fatalf("summary = %q", diags.Errors()[0].Summary())
	}
	_, diags = d.create(mk("trash.retention_days", "ninety"))
	mustErr(t, diags, "is an integer")
	_, diags = d.create(mk("search.typo_tolerance", "yes"))
	mustErr(t, diags, "\"true\" or \"false\"")
	_, diags = d.create(mk("mail.digest", "hourly"))
	mustErr(t, diags, "daily, weekly")
	_, diags = d.create(mk("no.such.key", "1"))
	mustErr(t, diags, "not_found")

	mustNoErr(t, d.delete(st))
	if rows["search.typo_tolerance"]["isDefault"] != true {
		t.Fatal("destroy must reset the setting to its default")
	}
}

func num(n int64) any { return stdjson.Number(strconv.FormatInt(n, 10)) }

func TestConnectorSecretsAreWriteOnly(t *testing.T) {
	f := newFakeAPI(t)
	stored := map[string]any{"clientId": "cid"}
	secrets := map[string]bool{}
	enabled := true
	updatedAt := any(nil)
	f.on("GET /orgs/org1/service-connectors/catalog", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{map[string]any{"provider": "zoom", "fields": []any{
			map[string]any{"key": "clientId", "kind": "text"}, map[string]any{"key": "clientSecret", "kind": "secret"},
		}}}}
	})
	row := func() map[string]any {
		names := []any{}
		for k := range secrets {
			names = append(names, k)
		}
		return map[string]any{"provider": "zoom", "name": "Zoom", "enabled": enabled, "configured": len(names) > 0, "connected": false, "connectedAccount": nil,
			"lastError": nil, "callbackUrl": "https://x.test/cb", "secretsSet": names, "updatedAt": updatedAt, "values": stored}
	}
	f.on("PUT /orgs/org1/service-connectors/zoom", 200, func(r *http.Request, b map[string]any) any {
		updatedAt = "2027-01-01T00:00:00Z"
		if v, ok := b["enabled"].(bool); ok {
			enabled = v
		}
		for k, v := range b["values"].(map[string]any) {
			switch {
			case k == "clientSecret" && v == nil:
				delete(secrets, k)
			case k == "clientSecret":
				secrets[k] = true
			case v == nil:
				delete(stored, k)
			default:
				stored[k] = v
			}
		}
		return row()
	})
	f.on("GET /orgs/org1/service-connectors/zoom", 200, func(*http.Request, map[string]any) any { return row() })
	f.on("DELETE /orgs/org1/service-connectors/zoom", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })

	d := newDriver[serviceConnectorModel](t, f, NewServiceConnectorResource)
	plan := serviceConnectorModel{
		ID: types.StringUnknown(), Provider: S("zoom"), Enabled: types.BoolUnknown(),
		Values:       types.MapValueMust(types.StringType, map[string]attr.Value{"clientId": S("cid")}),
		SecretValues: types.MapValueMust(types.StringType, map[string]attr.Value{"clientSecret": S("shh")}),
		Name:         types.StringUnknown(), Configured: types.BoolUnknown(), Connected: types.BoolUnknown(), ConnectedAccount: types.StringUnknown(),
		SecretsSet: types.SetUnknown(types.StringType), LastError: types.StringUnknown(), CallbackURL: types.StringUnknown(),
	}
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	body := f.find("PUT", "/orgs/org1/service-connectors/zoom")[0].Body
	vals := body["values"].(map[string]any)
	if vals["clientId"] != "cid" || vals["clientSecret"] != "shh" {
		t.Fatalf("body = %v", body)
	}
	if state.SecretValues.Elements()["clientSecret"].(types.String).ValueString() != "shh" {
		t.Fatal("the configured secret must stay in state exactly as configured (the API never returns it)")
	}
	if len(state.SecretsSet.Elements()) != 1 || !state.Configured.ValueBool() || state.CallbackURL.ValueString() == "" {
		t.Fatalf("state = %+v", state)
	}

	// A value changed in the console is drift; a secret cannot be read, so it is not.
	stored["clientId"] = "other"
	rd, _, diags := d.read(state)
	mustNoErr(t, diags)
	if rd.Values.Elements()["clientId"].(types.String).ValueString() != "other" {
		t.Fatal("values drift not detected")
	}
	if rd.SecretValues.Elements()["clientSecret"].(types.String).ValueString() != "shh" {
		t.Fatal("reading must not touch secret_values")
	}
	stored["clientId"] = "cid"

	// Removing the secret from configuration clears it.
	up := state
	up.SecretValues = types.MapNull(types.StringType)
	f.reset()
	_, diags = d.update(up, state)
	mustNoErr(t, diags)
	if v, has := f.find("PUT", "/orgs/org1/service-connectors/zoom")[0].Body["values"].(map[string]any)["clientSecret"]; !has || v != nil {
		t.Fatal("a removed secret must be sent as null")
	}

	// A secret in values (or the reverse) is refused with the attribute named.
	bad := plan
	bad.Values = types.MapValueMust(types.StringType, map[string]attr.Value{"clientSecret": S("oops")})
	bad.SecretValues = types.MapNull(types.StringType)
	_, diags = d.create(bad)
	mustErr(t, diags, "put it in secret_values")
	bad2 := plan
	bad2.Values = types.MapValueMust(types.StringType, map[string]attr.Value{"nope": S("x")})
	_, diags = d.create(bad2)
	mustErr(t, diags, "not a field of the zoom connector")

	mustNoErr(t, d.delete(state))
	if errs := d.attrErrors("provider_name", S("slack")); len(errs) == 0 {
		t.Error("provider enum")
	}
}

func TestSpaceLocaleCRUD(t *testing.T) {
	f := newFakeAPI(t)
	locales := []any{map[string]any{"code": "default", "label": "Original", "isDefault": true}}
	f.on("POST /orgs/org1/spaces/sp-1/docs/locales", 201, func(r *http.Request, b map[string]any) any {
		locales = append(locales, map[string]any{"code": b["code"], "label": b["label"], "isDefault": false})
		return map[string]any{"locales": locales}
	})
	f.on("GET /orgs/org1/spaces/sp-1/docs/locales", 200, func(*http.Request, map[string]any) any { return map[string]any{"data": locales} })
	f.on("PATCH /orgs/org1/spaces/sp-1/docs/locales/{code}", 200, func(r *http.Request, b map[string]any) any {
		for _, l := range locales {
			if l.(map[string]any)["code"] == r.PathValue("code") {
				l.(map[string]any)["label"] = b["label"]
			}
		}
		return map[string]any{"locales": locales}
	})
	f.on("DELETE /orgs/org1/spaces/sp-1/docs/locales/{code}", 200, func(r *http.Request, b map[string]any) any {
		out := []any{}
		for _, l := range locales {
			if l.(map[string]any)["code"] != r.PathValue("code") {
				out = append(out, l)
			}
		}
		locales = out
		return map[string]any{"locales": locales}
	})
	d := newDriver[spaceLocaleModel](t, f, NewSpaceLocaleResource)
	st, diags := d.create(spaceLocaleModel{ID: types.StringUnknown(), SpaceID: S("sp-1"), Code: S("es"), Label: S("Español"), IsDefault: types.BoolUnknown()})
	mustNoErr(t, diags)
	if st.ID.ValueString() != "sp-1/es" || st.IsDefault.ValueBool() {
		t.Fatalf("state = %+v", st)
	}
	up := st
	up.Label = S("Castellano")
	st2, diags := d.update(up, st)
	mustNoErr(t, diags)
	if st2.Label.ValueString() != "Castellano" {
		t.Fatal("relabel")
	}
	// The source language is relabelled, never created or deleted.
	def, diags := d.create(spaceLocaleModel{ID: types.StringUnknown(), SpaceID: S("sp-1"), Code: S("default"), Label: S("English"), IsDefault: types.BoolUnknown()})
	mustNoErr(t, diags)
	if len(f.find("POST", "/orgs/org1/spaces/sp-1/docs/locales")) != 1 || !def.IsDefault.ValueBool() {
		t.Fatal("default must be relabelled with PATCH, not created")
	}
	f.reset()
	mustNoErr(t, d.delete(def))
	if len(f.find("DELETE", "/orgs/org1/spaces/sp-1/docs/locales/default")) != 0 {
		t.Fatal("the source language cannot be deleted")
	}
	mustNoErr(t, d.delete(st2))
	_, gone, diags := d.read(st2)
	mustNoErr(t, diags)
	if !gone {
		t.Fatal("a removed language must leave state")
	}
	for v, ok := range map[string]bool{"es": true, "pt-br": true, "default": true, "E": false, "xx_yy": false, "ES": false} {
		if errs := d.attrErrors("code", S(v)); (len(errs) == 0) != ok {
			t.Errorf("code %q: %v", v, errs)
		}
	}
}
