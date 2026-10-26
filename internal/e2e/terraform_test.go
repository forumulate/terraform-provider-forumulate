// Package e2e runs the provider under a real Terraform CLI against a stateful
// fake of the API. Unit tests drive the resource interfaces directly; this is
// the check that Terraform's own consistency rules (a value planned must be the
// value applied, a second plan after an apply must be empty) hold for the
// whole provider.
//
// It needs a terraform binary: set FORUMULATE_E2E_TERRAFORM to its path. CI
// installs one; without it the test skips.
package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type tf struct {
	t    *testing.T
	bin  string
	dir  string
	env  []string
	last string
}

func (x *tf) run(args ...string) (string, int) {
	x.t.Helper()
	cmd := exec.Command(x.bin, args...)
	cmd.Dir = x.dir
	cmd.Env = append(os.Environ(), x.env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		x.t.Fatalf("terraform %v: %v", args, err)
	}
	x.last = out.String()
	return x.last, code
}

func (x *tf) mustRun(args ...string) string {
	x.t.Helper()
	out, code := x.run(args...)
	if code != 0 {
		x.t.Fatalf("terraform %v exited %d:\n%s", args, code, out)
	}
	return out
}

// planClean runs a plan and fails unless it proposes nothing, apart from the
// domain that is waiting for DNS: `activate = true` against an unverified name
// is work the next apply retries, so it stays in the plan by design.
func (x *tf) planClean(why string) {
	x.t.Helper()
	out, code := x.run("plan", "-detailed-exitcode", "-no-color", "-input=false")
	if code == 0 {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") && strings.Contains(line, " will be ") && !strings.HasPrefix(line, "# forumulate_domain.docs will be updated in-place") {
			x.t.Fatalf("%s: plan is not empty (exit %d):\n%s", why, code, out)
		}
	}
	if code != 2 {
		x.t.Fatalf("%s: plan failed (exit %d):\n%s", why, code, out)
	}
}

func buildProvider(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "terraform-provider-forumulate")
	cmd := exec.Command("go", "build", "-o", out, "../..")
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b.String())
	}
	return dir
}

const mainTF = `
terraform {
  required_providers {
    forumulate = { source = "forumulate/forumulate" }
  }
}

provider "forumulate" {}

data "forumulate_org" "this" {}

resource "forumulate_space" "docs" {
  slug       = "docs"
  name       = "Docs"
  type       = "DOCS"
  visibility = "PUBLIC"
  translations_json = jsonencode({ es = { name = "Documentación" } })
  settings_json     = jsonencode({ analytics = { id = "G-1" } })
}

resource "forumulate_space" "community" {
  slug = "community"
  name = "Community"
  type = "FORUM"
}

resource "forumulate_collection" "guides" {
  space_id    = forumulate_space.docs.id
  name        = "Guides"
  description = "Start here."
  nav = {
    icon      = "book"
    collapsed = true
  }
}

resource "forumulate_page" "quickstart" {
  space_id      = forumulate_space.docs.id
  collection_id = forumulate_collection.guides.id
  slug          = "quickstart"
  title         = "Quickstart"
  body          = "* one\n* two\n"
  metadata = {
    tags      = ["intro"]
    seo_title = "Quickstart | Docs"
  }
  metadata_json = jsonencode({ layout = "wide" })
}

resource "forumulate_forum_category" "general" {
  space_id    = forumulate_space.community.id
  name        = "General"
  description = "Talk about anything."
  color       = "0b835b"
  default_tags = ["welcome"]
  translations = {
    es = { name = "General", description = "Habla" }
  }
}

resource "forumulate_webhook" "publish" {
  url            = "https://hooks.example.com/forumulate"
  events         = ["page.published", "page.deleted"]
  space_id       = forumulate_space.docs.id
  alert_contacts = ["ops@example.com"]
}

resource "forumulate_domain" "docs" {
  space_id = forumulate_space.docs.id
  hostname = "docs.example.com"
  activate = true
  ip_allow_list = ["203.0.113.0/24"]
}

resource "forumulate_translation_override" "sign_in" {
  locale = "es"
  key    = "reader.sign_in"
  value  = "Entrar"
}

resource "forumulate_org_setting" "typo" {
  key   = "search.typo_tolerance"
  value = "false"
}

resource "forumulate_org_setting" "trash" {
  key   = "trash.retention_days"
  value = "60"
}

resource "forumulate_site_manifest" "site" {
  manifest_json = jsonencode({
    version = 2
    spaces = [
      { slug = "legal", name = "Legal", type = "LEGAL" },
    ]
  })
}

resource "forumulate_space_locale" "es" {
  space_id = forumulate_space.docs.id
  code     = "es"
  label    = "Español"
}

resource "forumulate_space_settings" "docs" {
  space_id   = forumulate_space.docs.id
  preset     = "standard"
  seo_json   = jsonencode({ seoTitle = "Docs", siteName = "Acme" })
  theme_json = jsonencode({ primaryColor = "#0b835b" })
  redirects = [
    { from = "/start", to = "/docs/getting-started", status = 301 },
    { from = "/old/*", to = "/new/*", pass_query = true },
  ]
}

resource "forumulate_site" "main" {
  slug        = "main"
  name        = "Acme"
  space_ids   = [forumulate_space.docs.id]
  config_json = jsonencode({ copyright = "© Acme", headerLinks = [{ label = "Docs", href = "/docs" }] })
  page_mounts = [
    { path = "/pricing", space_id = forumulate_space.docs.id, page = "pricing" },
  ]
}

resource "forumulate_service_connector" "zoom" {
  provider_name = "zoom"
  values        = { clientId = "cid" }
  secret_values = { clientSecret = "shh" }
}

resource "forumulate_connector" "intercom" {
  space_id          = forumulate_space.docs.id
  provider_id       = "intercom"
  config            = { workspaceId = "abc" }
  embed_enabled     = true
  knowledge_enabled = true
  knowledge_scopes  = ["search", "ask"]
  appearance = {
    color_scheme = "system"
    position     = "right"
  }
  mint_token = true
}

data "forumulate_space" "docs" {
  slug       = "docs"
  depends_on = [forumulate_space.docs]
}

output "org_slug"      { value = data.forumulate_org.this.slug }
output "docs_id"       { value = data.forumulate_space.docs.id }
output "webhook_secret" {
  value     = forumulate_webhook.publish.secret
  sensitive = true
}
output "dns_target"    { value = forumulate_domain.docs.dns_target.value }
output "legal_id"      { value = forumulate_site_manifest.site.ids["spaces/legal"] }
`

func TestProviderUnderRealTerraform(t *testing.T) {
	bin := os.Getenv("FORUMULATE_E2E_TERRAFORM")
	if bin == "" {
		t.Skip("set FORUMULATE_E2E_TERRAFORM to a terraform binary to run the end-to-end test")
	}
	f := newFake(t)
	providerDir := buildProvider(t)
	work := t.TempDir()
	rc := filepath.Join(work, "terraformrc")
	if err := os.WriteFile(rc, []byte(`provider_installation {
  dev_overrides { "forumulate/forumulate" = "`+providerDir+`" }
  direct {}
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "main.tf"), []byte(mainTF), 0o600); err != nil {
		t.Fatal(err)
	}
	x := &tf{t: t, bin: bin, dir: work, env: []string{
		"TF_CLI_CONFIG_FILE=" + rc, "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1",
		"FORUMULATE_TOKEN=fp_e2e", "FORUMULATE_ORG_ID=o1", "FORUMULATE_API_URL=" + f.srv.URL,
	}}

	// validate: the configuration type-checks against every schema.
	x.mustRun("validate", "-no-color")

	// apply creates everything, in dependency order, with no inconsistency errors.
	out := x.mustRun("apply", "-auto-approve", "-no-color", "-input=false")
	if !strings.Contains(out, "Apply complete!") {
		t.Fatalf("apply output:\n%s", out)
	}
	// The domain's DNS is not published, so activation warns instead of failing.
	if !strings.Contains(out, "Domain not verified yet") {
		t.Errorf("expected the not-verified warning:\n%s", out)
	}
	if len(f.spaces) != 3 || len(f.collections) != 1 || len(f.pages) != 1 || len(f.categories) != 1 || len(f.webhooks) != 1 || len(f.domains) != 1 {
		t.Fatalf("remote state: %d spaces %d collections %d pages %d categories %d webhooks %d domains", len(f.spaces), len(f.collections), len(f.pages), len(f.categories), len(f.webhooks), len(f.domains))
	}
	if len(f.spaceConnectors) != 1 {
		t.Fatalf("space connectors: %d", len(f.spaceConnectors))
	}
	if len(f.sites) != 1 || len(f.locales) != 1 || f.connector == nil || len(f.redirects) != 1 {
		t.Fatalf("remote state: %d sites %d locales connector %v redirects %v", len(f.sites), len(f.locales), f.connector, f.redirects)
	}
	for _, p := range f.pages {
		if p["status"] != "published" {
			t.Errorf("the page must be published, is %v", p["status"])
		}
		if f.bodies[p["id"].(string)] != "- one\n- two\n" {
			t.Errorf("the fake normalises the body; stored %q", f.bodies[p["id"].(string)])
		}
	}

	// The second plan is empty: nothing planned differs from what was applied,
	// the API's rewriting of the Markdown and of the colour is not drift, and
	// the manifest's dry run finds nothing to do.
	x.planClean("after the first apply")

	// Outputs.
	outputs := map[string]struct {
		Value any `json:"value"`
	}{}
	if err := json.Unmarshal([]byte(x.mustRun("output", "-json")), &outputs); err != nil {
		t.Fatal(err)
	}
	if outputs["org_slug"].Value != "acme" || outputs["dns_target"].Value != "edge.forumulate.test" || outputs["legal_id"].Value == nil {
		t.Fatalf("outputs = %v", outputs)
	}
	if s, _ := outputs["webhook_secret"].Value.(string); !strings.HasPrefix(s, "whsec_") {
		t.Fatalf("webhook secret = %v", outputs["webhook_secret"])
	}

	// Drift: a collection renamed in the console shows in the plan and the next
	// apply puts it back; so does a space created by the manifest.
	for _, c := range f.collections {
		c["name"] = "Renamed in the studio"
	}
	out, code := x.run("plan", "-detailed-exitcode", "-no-color", "-input=false")
	if code != 2 || !strings.Contains(out, "forumulate_collection.guides will be updated in-place") || !strings.Contains(out, "Renamed in the studio") {
		t.Fatalf("drift not planned (exit %d):\n%s", code, out)
	}
	x.mustRun("apply", "-auto-approve", "-no-color", "-input=false")
	x.planClean("after fixing the collection drift")

	// A studio edit to the page body is drift too.
	for id := range f.pages {
		f.bodies[id] = "- one\n- two\n- three (added in the studio)\n"
	}
	out, code = x.run("plan", "-detailed-exitcode", "-no-color", "-input=false")
	if code != 2 || !strings.Contains(out, "forumulate_page.quickstart will be updated in-place") {
		t.Fatalf("page body drift not planned (exit %d):\n%s", code, out)
	}
	x.mustRun("apply", "-auto-approve", "-no-color", "-input=false")
	x.planClean("after fixing the page drift")

	// A space deleted outside Terraform is recreated, not an error.
	for id, s := range f.spaces {
		if s["slug"] == "community" {
			delete(f.spaces, id)
		}
	}
	for id := range f.categories {
		delete(f.categories, id)
	}
	out, code = x.run("plan", "-detailed-exitcode", "-no-color", "-input=false")
	if code != 2 || !strings.Contains(out, "forumulate_space.community will be created") {
		t.Fatalf("a space deleted outside Terraform must be planned for creation (exit %d):\n%s", code, out)
	}
	x.mustRun("apply", "-auto-approve", "-no-color", "-input=false")
	x.planClean("after recreating the space")

	// The manifest plan names what it would do before it does it.
	if err := os.WriteFile(filepath.Join(work, "main.tf"), []byte(strings.Replace(mainTF,
		`{ slug = "legal", name = "Legal", type = "LEGAL" }`, `{ slug = "legal", name = "Legal & Trust", type = "LEGAL" }, { slug = "careers", name = "Careers", type = "CAREERS" }`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code = x.run("plan", "-detailed-exitcode", "-no-color", "-input=false")
	if code != 2 || !strings.Contains(out, "Manifest plan: 1 create, 1 update") || !strings.Contains(out, "create spaces/careers") || !strings.Contains(out, "update spaces/legal") {
		t.Fatalf("manifest plan not shown (exit %d):\n%s", code, out)
	}
	x.mustRun("apply", "-auto-approve", "-no-color", "-input=false")
	x.planClean("after the manifest change")

	// A configuration the plan-time validators refuse never reaches the API.
	before := len(f.requests)
	if err := os.WriteFile(filepath.Join(work, "bad.tf"), []byte(`
resource "forumulate_space" "bad" {
  slug = "Not A Slug"
  name = "Bad"
  type = "WIKI"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code = x.run("validate", "-no-color")
	if code == 0 || !strings.Contains(out, "lowercase letters and digits separated by single") || !strings.Contains(out, "WIKI") {
		t.Fatalf("validators did not refuse the space (exit %d):\n%s", code, out)
	}
	if len(f.requests) != before {
		t.Fatal("validation must not call the API")
	}
	_ = os.Remove(filepath.Join(work, "bad.tf"))

	// Import: a space by slug.
	x.mustRun("state", "rm", "forumulate_space.community")
	x.mustRun("import", "-no-color", "-input=false", "forumulate_space.community", "community")
	x.planClean("after importing a space")

	// Destroy removes everything.
	x.mustRun("destroy", "-auto-approve", "-no-color", "-input=false")
	if len(f.collections) != 0 || len(f.webhooks) != 0 || len(f.domains) != 0 || len(f.pages) != 0 || len(f.overrides) != 0 {
		t.Fatalf("destroy left remote objects: %d collections %d webhooks %d domains %d pages %d overrides", len(f.collections), len(f.webhooks), len(f.domains), len(f.pages), len(f.overrides))
	}
	if _, set := f.settings["search.typo_tolerance"]; set {
		t.Fatal("destroying an org setting must reset it")
	}
}
