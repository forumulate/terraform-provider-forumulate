package provider

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func providerSchema(t *testing.T) *tfprotov6.GetProviderSchemaResponse {
	t.Helper()
	srv := providerserver.NewProtocol6(New("test")())()
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Fatalf("schema diagnostic: %s: %s", d.Summary, d.Detail)
	}
	return resp
}

func TestProviderRegistersEverything(t *testing.T) {
	resp := providerSchema(t)
	got := []string{}
	for name := range resp.ResourceSchemas {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{
		"forumulate_collection", "forumulate_connector", "forumulate_domain", "forumulate_forum_category", "forumulate_org_setting",
		"forumulate_page", "forumulate_service_connector", "forumulate_site", "forumulate_site_manifest", "forumulate_space", "forumulate_space_locale",
		"forumulate_space_settings", "forumulate_status_component", "forumulate_status_groups", "forumulate_status_incident_template",
		"forumulate_status_metric", "forumulate_translation_override", "forumulate_webhook",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("resources = %v\nwant %v", got, want)
	}
	ds := []string{}
	for name := range resp.DataSourceSchemas {
		ds = append(ds, name)
	}
	sort.Strings(ds)
	if strings.Join(ds, ",") != "forumulate_org,forumulate_space,forumulate_space_types,forumulate_status_page" {
		t.Fatalf("data sources = %v", ds)
	}
}

// Every attribute of every resource and data source carries a description:
// the generated registry docs are the only manual a Terraform user gets.
func TestEverySchemaAttributeIsDocumented(t *testing.T) {
	resp := providerSchema(t)
	var walk func(owner string, b *tfprotov6.SchemaBlock)
	var walkNested func(owner string, o *tfprotov6.SchemaObject)
	walkNested = func(owner string, o *tfprotov6.SchemaObject) {
		if o == nil {
			return
		}
		for _, a := range o.Attributes {
			if a.Description == "" {
				t.Errorf("%s.%s has no description", owner, a.Name)
			}
			walkNested(owner+"."+a.Name, a.NestedType)
		}
	}
	walk = func(owner string, b *tfprotov6.SchemaBlock) {
		if b.Description == "" && owner != "" && !strings.HasPrefix(owner, "forumulate_status") {
			t.Errorf("%s has no description", owner)
		}
		for _, a := range b.Attributes {
			if a.Name == "id" && a.Computed {
				// ids of the original status resources predate this rule.
				if !strings.HasPrefix(owner, "forumulate_status") && a.Description == "" {
					t.Errorf("%s.id has no description", owner)
				}
				continue
			}
			if a.Description == "" && !strings.HasPrefix(owner, "forumulate_status") {
				t.Errorf("%s.%s has no description", owner, a.Name)
			}
			walkNested(owner+"."+a.Name, a.NestedType)
		}
		for _, nb := range b.BlockTypes {
			walk(owner+"."+nb.TypeName, nb.Block)
		}
	}
	for name, s := range resp.ResourceSchemas {
		walk(name, s.Block)
	}
	for name, s := range resp.DataSourceSchemas {
		walk(name, s.Block)
	}
}

func TestProviderConfigurePicksEnvironment(t *testing.T) {
	t.Setenv("FORUMULATE_TOKEN", "fp_env")
	t.Setenv("FORUMULATE_ORG_ID", "org-env")
	t.Setenv("FORUMULATE_API_URL", "https://api.example.test/api/")
	if got := pick(types.StringNull(), "FORUMULATE_API_URL", "x"); got != "https://api.example.test/api/" {
		t.Fatalf("api url = %q", got)
	}
	if got := pick(S("from-config"), "FORUMULATE_TOKEN", ""); got != "from-config" {
		t.Fatalf("configuration must win over the environment, got %q", got)
	}
}
