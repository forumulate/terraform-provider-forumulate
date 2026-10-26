package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func jn(s string) json.Number { return json.Number(s) }

func mustParse(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := decodeJSON(s, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJSONFromAPIComparesOnlyWhatWasDeclared(t *testing.T) {
	api := mustParse(t, `{"a":1,"b":{"c":"x","d":[1,2]},"extra":"added by the API"}`)

	// Same meaning, different spelling and key order: the configured text stays.
	prior := S(`{ "b": {"c": "x"}, "a": 1.0 }`)
	if got := jsonFromAPI(prior, api); got.ValueString() != prior.ValueString() {
		t.Fatalf("got %s", got.ValueString())
	}
	// A declared nested key changed: the API's value for the declared shape shows.
	drift := S(`{"b":{"c":"y"}}`)
	if got := jsonFromAPI(drift, api); got.ValueString() != `{"b":{"c":"x"}}` {
		t.Fatalf("got %s", got.ValueString())
	}
	// A declared key that vanished is drift too.
	if got := jsonFromAPI(S(`{"gone":true}`), api); got.ValueString() != `{}` {
		t.Fatalf("got %s", got.ValueString())
	}
	// A key declared as null means "absent": absence matches.
	if got := jsonFromAPI(S(`{"nope":null}`), api); got.ValueString() != `{"nope":null}` {
		t.Fatalf("got %s", got.ValueString())
	}
	// Not declared at all: whatever the API holds is not drift.
	if got := jsonFromAPI(types.StringNull(), api); !got.IsNull() {
		t.Fatalf("got %v", got)
	}
	// Arrays are compared whole.
	if got := jsonFromAPI(S(`{"b":{"d":[2,1]}}`), api); got.ValueString() != `{"b":{"d":[1,2]}}` {
		t.Fatalf("got %s", got.ValueString())
	}
}

func TestNullRemovedClearsDroppedKeysAtAnyDepth(t *testing.T) {
	prior := mustParse(t, `{"a":1,"b":{"c":1,"d":2},"keep":true}`)
	plan := mustParse(t, `{"b":{"c":1},"keep":true,"new":1}`)
	got := canonical(nullRemoved(prior, plan))
	if got != `{"a":null,"b":{"c":1,"d":null},"keep":true,"new":1}` {
		t.Fatalf("deep = %s", got)
	}
	if got := canonical(nullRemovedTop(prior, plan)); got != `{"a":null,"b":{"c":1},"keep":true,"new":1}` {
		t.Fatalf("top = %s", got)
	}
}

func TestMergeBodyBuildsPatchDocuments(t *testing.T) {
	b := Object{}
	mergeBody(b, "k", S(`{"a":1,"b":2}`), S(`{"a":3}`))
	if canonical(b["k"]) != `{"a":3,"b":null}` {
		t.Fatalf("got %s", canonical(b["k"]))
	}
	// The whole attribute removed: every key it had is cleared.
	b = Object{}
	mergeBody(b, "k", S(`{"a":1}`), types.StringNull())
	if canonical(b["k"]) != `{"a":null}` {
		t.Fatalf("got %s", canonical(b["k"]))
	}
	// Create: nothing to clear.
	b = Object{}
	mergeBody(b, "k", types.StringNull(), S(`{"a":1}`))
	if canonical(b["k"]) != `{"a":1}` {
		t.Fatalf("got %s", canonical(b["k"]))
	}
	// Nothing declared, nothing sent.
	b = Object{}
	mergeBody(b, "k", types.StringNull(), types.StringNull())
	if len(b) != 0 {
		t.Fatalf("got %v", b)
	}
}

func TestNestedObjectsReadOnlyDeclaredFields(t *testing.T) {
	fields := []fieldMap{{"a", "a", kStr}, {"deep", "x.y", kInt}, {"tags", "tags", kStrSet}}
	at := attrTypesOf(fields)
	prior, _ := types.ObjectValue(at, map[string]attr.Value{"a": S("one"), "deep": types.Int64Null(), "tags": setOfStrings([]string{"t"})})
	api := Object{"a": "two", "x": map[string]any{"y": jn("5")}, "tags": []any{"t", "u"}}
	got := objectFromAPI(prior, fields, api, false).Attributes()
	if got["a"].(types.String).ValueString() != "two" || !got["deep"].IsNull() || len(got["tags"].(types.Set).Elements()) != 2 {
		t.Fatalf("got %v", got)
	}
	// Unmanaged: null, whatever the API holds.
	if !objectFromAPI(types.ObjectNull(at), fields, api, false).IsNull() {
		t.Fatal("an object that was not declared must stay null")
	}
	// Body: dotted keys nest, dropped fields clear.
	planObj, _ := types.ObjectValue(at, map[string]attr.Value{"a": types.StringNull(), "deep": types.Int64Value(3), "tags": types.SetNull(types.StringType)})
	body := objectBody(planObj, fields, &prior)
	if canonical(body) != `{"a":null,"tags":null,"x":{"y":3}}` {
		t.Fatalf("body = %s", canonical(body))
	}
}

func TestAPIErrorsAreMappedOntoAttributes(t *testing.T) {
	attrs := []string{"config_json", "space_ids", "page_mounts", "name"}
	alias := map[string]string{"config": "config_json"}
	for param, want := range map[string]string{
		"config.pageMounts.0.path": "page_mounts", "config.header.height": "config_json", "spaceIds": "space_ids", "pageMounts": "page_mounts", "name": "name",
		"body.name": "name", "unknown": "", "": "",
	} {
		if got := attrFor(param, attrs, alias); got != want {
			t.Errorf("attrFor(%q) = %q, want %q", param, got, want)
		}
	}
	if snake("externalRef") != "external_ref" || snake("who") != "who" {
		t.Fatal("snake")
	}
}
