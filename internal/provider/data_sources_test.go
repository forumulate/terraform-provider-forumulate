package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// readData runs a data source's Read over a config built from m.
func readData[M any](t *testing.T, f *fakeAPI, ds datasource.DataSource, cfg M) (M, []string) {
	t.Helper()
	ctx := context.Background()
	var sresp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &sresp)
	if d := sresp.Schema.ValidateImplementation(ctx); d.HasError() {
		t.Fatalf("schema invalid: %v", d)
	}
	ds.(datasource.DataSourceWithConfigure).Configure(ctx, datasource.ConfigureRequest{ProviderData: f.client()}, &datasource.ConfigureResponse{})
	config := tfsdk.Config{Schema: sresp.Schema, Raw: tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil)}
	st := tfsdk.State{Schema: sresp.Schema, Raw: config.Raw}
	if d := st.Set(ctx, &cfg); d.HasError() {
		t.Fatal(d)
	}
	config.Raw = st.Raw
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: sresp.Schema, Raw: tftypes.NewValue(sresp.Schema.Type().TerraformType(ctx), nil)}}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, &resp)
	var errs []string
	for _, e := range resp.Diagnostics.Errors() {
		errs = append(errs, e.Summary()+": "+e.Detail())
	}
	var out M
	if len(errs) == 0 {
		if d := resp.State.Get(ctx, &out); d.HasError() {
			t.Fatal(d)
		}
	}
	return out, errs
}

func TestSpaceDataSourceFindsBySlug(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET /orgs/org1/spaces/docs", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"space": map[string]any{"id": "sp-1", "slug": "docs", "name": "Docs", "type": "DOCS", "visibility": "PUBLIC",
			"itemCount": 12, "createdAt": "2027-01-01T00:00:00Z", "settings": map[string]any{"b": 1, "a": "x"}}}
	})
	f.on("GET /orgs/org1/spaces/nope", 404, func(*http.Request, map[string]any) any {
		return fail(404, "space.not_found", "No such space.", "spaceId")
	})
	got, errs := readData(t, f, NewSpaceDataSource(), spaceDataModel{Slug: S("docs"),
		ID: types.StringNull(), Name: types.StringNull(), Type: types.StringNull(), Visibility: types.StringNull(),
		SettingsJSON: types.StringNull(), ItemCount: types.Int64Null(), CreatedAt: types.StringNull()})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if got.ID.ValueString() != "sp-1" || got.Type.ValueString() != "DOCS" || got.ItemCount.ValueInt64() != 12 || got.SettingsJSON.ValueString() != `{"a":"x","b":1}` {
		t.Fatalf("space = %+v", got)
	}
	_, errs = readData(t, f, NewSpaceDataSource(), spaceDataModel{Slug: S("nope"),
		ID: types.StringNull(), Name: types.StringNull(), Type: types.StringNull(), Visibility: types.StringNull(),
		SettingsJSON: types.StringNull(), ItemCount: types.Int64Null(), CreatedAt: types.StringNull()})
	if len(errs) != 1 || errs[0] != `No such space: The organization has no space "nope".` {
		t.Fatalf("errs = %v", errs)
	}
}

func TestOrgDataSource(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET /orgs/org1", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"organization": map[string]any{"id": "o-uuid", "name": "Acme", "slug": "acme", "contentLanguage": "en", "inboundEmailDomain": nil,
			"trashRetentionDays": 30, "createdAt": "2027-01-01T00:00:00Z"}}
	})
	got, errs := readData(t, f, NewOrgDataSource(), orgDataModel{ID: types.StringNull(), Name: types.StringNull(), Slug: types.StringNull(),
		ContentLanguage: types.StringNull(), InboundEmailDomain: types.StringNull(), TrashRetentionDays: types.Int64Null(), CreatedAt: types.StringNull()})
	if len(errs) != 0 || got.Slug.ValueString() != "acme" || got.TrashRetentionDays.ValueInt64() != 30 || !got.InboundEmailDomain.IsNull() {
		t.Fatalf("org = %+v %v", got, errs)
	}
}

func TestSpaceTypesDataSource(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET /space-types", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{
			map[string]any{"type": "BLOG", "name": "Blog", "description": "d", "purpose": "p", "backing": "page",
				"vocabulary": map[string]any{"piece": []any{"article", "post"}, "container": []any{"tag"}},
				"route":      map[string]any{"defaultSlug": "blog"}, "tabs": []any{"content", "settings"},
				"presets":      []any{map[string]any{"name": "classic", "recommended": true}, map[string]any{"name": "magazine"}},
				"piecePresets": []any{map[string]any{"name": "narrow"}}},
			map[string]any{"type": "STATUS", "name": "Status", "forcedVisibility": "PRIVATE", "vocabulary": map[string]any{}, "route": map[string]any{}},
		}}
	})
	got, errs := readData(t, f, NewSpaceTypesDataSource(), spaceTypesModel{ID: types.StringNull(), Names: types.ListNull(types.StringType),
		Types: types.ListNull(types.ObjectType{AttrTypes: spaceTypeAttrTypes})})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if n := got.Names.Elements(); len(n) != 2 || n[0].(types.String).ValueString() != "BLOG" {
		t.Fatalf("names = %v", n)
	}
	blog := got.Types.Elements()[0].(types.Object).Attributes()
	if blog["default_slug"].(types.String).ValueString() != "blog" || blog["recommended_preset"].(types.String).ValueString() != "classic" ||
		len(blog["presets"].(types.List).Elements()) != 2 || len(blog["piece_names"].(types.List).Elements()) != 2 {
		t.Fatalf("blog = %v", blog)
	}
	if got.Types.Elements()[1].(types.Object).Attributes()["forced_visibility"].(types.String).ValueString() != "PRIVATE" {
		t.Fatal("forced visibility")
	}
}
