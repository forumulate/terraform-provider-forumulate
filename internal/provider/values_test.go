package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProbeStateKeepsTheConfiguredSpelling(t *testing.T) {
	prior := types.StringValue(`{ "port": 5432, "host": "db.example.com" }`)
	got := probeState(prior, json.RawMessage(`{"host":"db.example.com","port":5432}`))
	if !got.Equal(prior) {
		t.Fatalf("same probe read as drift: %v", got)
	}
	changed := probeState(prior, json.RawMessage(`{"host":"db2.example.com","port":5432}`))
	if changed.ValueString() != `{"host":"db2.example.com","port":5432}` {
		t.Fatalf("changed probe = %v", changed)
	}
	if !probeState(prior, json.RawMessage(`null`)).IsNull() {
		t.Fatal("a null probe should read null")
	}
}

func TestComponentBodyClearsDroppedFieldsOnUpdate(t *testing.T) {
	m := componentModel{
		Name:      types.StringValue("API"),
		Kind:      types.StringValue("tcp"),
		Group:     types.StringNull(),
		BarDays:   types.Int64Value(30),
		Regions:   types.ListNull(types.StringType),
		ProbeJSON: types.StringValue(`{"host":"db","port":5432}`),
		Paused:    types.BoolUnknown(),
	}
	create, d := componentBody(context.Background(), m, false)
	if d.HasError() {
		t.Fatal(d)
	}
	if create["kind"] != "tcp" || create["barDays"] != int64(30) || create["probe"].(map[string]any)["host"] != "db" {
		t.Fatalf("create body = %v", create)
	}
	if _, ok := create["group"]; ok {
		t.Fatal("create should not send an unset group")
	}
	if _, ok := create["paused"]; ok {
		t.Fatal("an unknown computed value is not sent")
	}
	update, _ := componentBody(context.Background(), m, true)
	if v, ok := update["group"]; !ok || v != nil {
		t.Fatalf("update should clear the group: %v", update)
	}
	if _, ok := update["kind"]; ok {
		t.Fatal("kind is fixed at creation and never patched")
	}
	m.ProbeJSON = types.StringValue(`not json`)
	if _, d := componentBody(context.Background(), m, false); !d.HasError() {
		t.Fatal("bad probe_json should be an error")
	}
}

func TestFromGroupsIgnoresUnarrangedGroupsItNeverManaged(t *testing.T) {
	desc := "Core"
	m := groupsModel{Groups: []groupModel{{Name: types.StringValue("Core"), Collapsed: types.BoolNull()}}}
	fromGroups(&m, []client.Group{{Name: "Core", Description: &desc}, {Name: "Stray"}})
	if len(m.Groups) != 1 || m.Groups[0].Description.ValueString() != "Core" || !m.Groups[0].Collapsed.IsNull() {
		t.Fatalf("groups = %+v", m.Groups)
	}
	imported := groupsModel{}
	fromGroups(&imported, []client.Group{{Name: "Core"}, {Name: "Stray"}})
	if len(imported.Groups) != 2 {
		t.Fatalf("an import keeps every group: %+v", imported.Groups)
	}
}

func TestTemplateBodyCarriesPerStateWords(t *testing.T) {
	b := templateBody(templateModel{
		Name: types.StringValue("DB"), Title: types.StringValue("Database"),
		Resolved: types.StringValue("Fixed {{name}}"), Identified: types.StringNull(), Monitoring: types.StringNull(),
		Markdown: types.StringNull(), Severity: types.StringUnknown(), State: types.StringNull(),
	})
	u := b["updates"].(map[string]string)
	if u["resolved"] != "Fixed {{name}}" || len(u) != 1 {
		t.Fatalf("updates = %v", u)
	}
	if _, ok := b["severity"]; ok {
		t.Fatal("unknown severity is not sent")
	}
}
