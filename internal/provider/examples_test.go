package provider

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// The examples are what the registry docs print and what a user copies, so
// they are checked against the schema: every resource and data source has
// one, every attribute and block it uses exists, and every required attribute
// is set. An example that drifts from the schema fails here instead of in a
// user's `terraform validate`.

func exampleSchemas(t *testing.T) (map[string]*tfprotov6.Schema, map[string]*tfprotov6.Schema) {
	resp := providerSchema(t)
	return resp.ResourceSchemas, resp.DataSourceSchemas
}

func parseHCL(t *testing.T, file string) *hclsyntax.Body {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	f, diags := hclsyntax.ParseConfig(src, file, hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("%s: %s", file, diags.Error())
	}
	return f.Body.(*hclsyntax.Body)
}

func checkBlock(t *testing.T, where string, body *hclsyntax.Body, block *tfprotov6.SchemaBlock) {
	t.Helper()
	attrs := map[string]*tfprotov6.SchemaAttribute{}
	for _, a := range block.Attributes {
		attrs[a.Name] = a
	}
	nested := map[string]*tfprotov6.SchemaNestedBlock{}
	for _, b := range block.BlockTypes {
		nested[b.TypeName] = b
	}
	for name, ba := range body.Attributes {
		sa, ok := attrs[name]
		if !ok {
			t.Errorf("%s: attribute %q does not exist in the schema", where, name)
			continue
		}
		if sa.Computed && !sa.Optional && !sa.Required {
			t.Errorf("%s: attribute %q is computed-only and cannot be set", where, name)
		}
		if sa.NestedType != nil {
			checkNested(t, where+"."+name, ba.Expr, sa.NestedType)
		}
	}
	for _, b := range body.Blocks {
		nb, ok := nested[b.Type]
		if !ok {
			t.Errorf("%s: block %q does not exist in the schema", where, b.Type)
			continue
		}
		checkBlock(t, where+"."+b.Type, b.Body, nb.Block)
	}
	for _, a := range block.Attributes {
		if a.Required {
			if _, set := body.Attributes[a.Name]; !set {
				t.Errorf("%s: required attribute %q is not set", where, a.Name)
			}
		}
	}
}

// checkNested checks the keys written inside a nested attribute against the
// nested object's schema: a single object, a list or set of objects, or a map
// of objects.
func checkNested(t *testing.T, where string, expr hclsyntax.Expression, obj *tfprotov6.SchemaObject) {
	t.Helper()
	attrs := map[string]*tfprotov6.SchemaAttribute{}
	for _, a := range obj.Attributes {
		attrs[a.Name] = a
	}
	checkObject := func(where string, e hclsyntax.Expression) {
		cons, ok := e.(*hclsyntax.ObjectConsExpr)
		if !ok {
			return // a reference or function call: nothing to check statically
		}
		set := map[string]bool{}
		for _, item := range cons.Items {
			key := hcl.ExprAsKeyword(item.KeyExpr)
			if key == "" {
				continue
			}
			set[key] = true
			sa, ok := attrs[key]
			if !ok {
				t.Errorf("%s: %q is not an attribute of this nested object", where, key)
				continue
			}
			if sa.NestedType != nil {
				checkNested(t, where+"."+key, item.ValueExpr, sa.NestedType)
			}
		}
		for _, a := range obj.Attributes {
			if a.Required && !set[a.Name] {
				t.Errorf("%s: required attribute %q is not set", where, a.Name)
			}
		}
	}
	switch obj.Nesting {
	case tfprotov6.SchemaObjectNestingModeSingle:
		checkObject(where, expr)
	case tfprotov6.SchemaObjectNestingModeList, tfprotov6.SchemaObjectNestingModeSet:
		if tup, ok := expr.(*hclsyntax.TupleConsExpr); ok {
			for i, e := range tup.Exprs {
				checkObject(where+"[]"+string(rune('0'+i)), e)
			}
		}
	case tfprotov6.SchemaObjectNestingModeMap:
		if cons, ok := expr.(*hclsyntax.ObjectConsExpr); ok {
			for _, item := range cons.Items {
				checkObject(where+"[map]", item.ValueExpr)
			}
		}
	}
}

func TestExamplesMatchTheSchema(t *testing.T) {
	resources, dataSources := exampleSchemas(t)
	root := filepath.Join("..", "..", "examples")

	seen := map[string]bool{}
	for _, kind := range []struct {
		dir, file, block string
		schemas          map[string]*tfprotov6.Schema
	}{
		{"resources", "resource.tf", "resource", resources},
		{"data-sources", "data-source.tf", "data", dataSources},
	} {
		names := make([]string, 0, len(kind.schemas))
		for n := range kind.schemas {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			file := filepath.Join(root, kind.dir, name, kind.file)
			if _, err := os.Stat(file); err != nil {
				t.Errorf("%s has no example at %s", name, file)
				continue
			}
			if kind.dir == "resources" {
				if _, err := os.Stat(filepath.Join(root, kind.dir, name, "import.sh")); err != nil {
					t.Errorf("%s has no import.sh", name)
				}
			}
			body := parseHCL(t, file)
			used := false
			for _, b := range body.Blocks {
				if b.Type != kind.block || len(b.Labels) != 2 {
					continue
				}
				schemaOf, ok := kind.schemas[b.Labels[0]]
				if !ok {
					t.Errorf("%s: unknown type %s", file, b.Labels[0])
					continue
				}
				seen[b.Labels[0]] = true
				if b.Labels[0] == name {
					used = true
				}
				checkBlock(t, file+" "+b.Labels[0]+"."+b.Labels[1], b.Body, schemaOf.Block)
			}
			if !used {
				t.Errorf("%s does not use %s", file, name)
			}
		}
	}
	// The provider example parses and configures only what exists.
	parseHCL(t, filepath.Join(root, "provider", "provider.tf"))
	// Every import.sh names the resource it imports.
	files, _ := filepath.Glob(filepath.Join(root, "resources", "*", "import.sh"))
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		typ := filepath.Base(filepath.Dir(f))
		if !strings.Contains(string(raw), "terraform import "+typ+".") {
			t.Errorf("%s does not import %s", f, typ)
		}
	}
}
