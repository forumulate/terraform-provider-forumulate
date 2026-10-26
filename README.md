# Terraform provider for Forumulate

Manage a [Forumulate](https://forumulate.com) organization as code: spaces and their pages, collections, forum
categories, sites, domains, webhooks, settings, connectors and status pages, or the whole site at once as one
manifest. It talks to the same REST API the console, the CLI and the SDKs use, authenticated with a personal
access token, so it can do exactly what the person holding the token can.

- Every resource has full create, read, update, delete and import, validates what it can at `terraform plan`
  (names, slugs, enums, lengths and counts match the API's own rules), detects drift on read, and shows API
  errors on the attribute they name, with the API's hint and request id.
- `forumulate_site_manifest` plans through the API's own dry run: `terraform plan` prints the manifest plan step by
  step, and `terraform apply` applies it.
- Documentation for every resource and data source: [`docs/`](docs/) (the Terraform Registry layout), with
  runnable [`examples/`](examples/).

## Install

**From the Terraform Registry** (once the provider is listed there):

```hcl
terraform {
  required_providers {
    forumulate = {
      source  = "forumulate/forumulate"
      version = "~> 0.2"
    }
  }
}
```

**Until it is listed**, install a release by hand. Every release on
[GitHub](https://github.com/forumulate/terraform-provider-forumulate/releases) carries a zip per platform
(`terraform-provider-forumulate_<version>_<os>_<arch>.zip`), a signed `SHA256SUMS` file and its signature.

```sh
VERSION=0.2.0 OS=darwin ARCH=arm64          # linux/amd64, windows/amd64, ... are published too
DIR=~/.terraform.d/plugins/registry.terraform.io/forumulate/forumulate/$VERSION/${OS}_${ARCH}
mkdir -p "$DIR"
curl -sSL -o /tmp/forumulate.zip \
  "https://github.com/forumulate/terraform-provider-forumulate/releases/download/v$VERSION/terraform-provider-forumulate_${VERSION}_${OS}_${ARCH}.zip"
unzip -o /tmp/forumulate.zip -d "$DIR"
```

Then tell Terraform to find it in that folder, in `~/.terraformrc` (`%APPDATA%\terraform.rc` on Windows):

```hcl
provider_installation {
  filesystem_mirror {
    path    = "~/.terraform.d/plugins"
    include = ["forumulate/forumulate"]
  }
  direct {
    exclude = ["forumulate/forumulate"]
  }
}
```

and use the same `required_providers` block as above; `terraform init` installs it from the folder. To try a build from source
instead, `make build` prints a `dev_overrides` block that points Terraform at it (no `init` needed).

## Authenticate

Create a personal access token in the console (Account, Tokens) and export it with your organization's id:

```sh
export FORUMULATE_TOKEN="fp_..."    # a personal access token; needs the write scope to change anything
export FORUMULATE_ORG_ID="..."      # the organization's id or slug
export FORUMULATE_API_URL="..."     # optional, defaults to https://forumulate.com/api
```

```hcl
provider "forumulate" {}
```

`token`, `org_id` and `api_url` can be set in the block too; prefer the environment so a token never lands in
configuration or version control. Tokens expire (90 days by default, a year at most): rotate the one in your
pipeline. Every request pins `Forumulate-Version: 2027-02-01`, so a taken page slug is an error rather than a silent suffix and a
field the API does not know is refused rather than ignored.

## What it manages

| Resource | What it manages | Import id |
| --- | --- | --- |
| [`forumulate_space`](docs/resources/space.md) | A space of any type: slug, name, type, visibility, translations, settings, tags. | `<slug or id>` |
| [`forumulate_space_locale`](docs/resources/space_locale.md) | A language a docs space publishes in. | `<space_id>/<code>` |
| [`forumulate_space_settings`](docs/resources/space_settings.md) | A space's layout, SEO, theme, redirect table and its type's own settings (blog, forum, status). | `<space slug or id>` |
| [`forumulate_collection`](docs/resources/collection.md) | A docs collection: nesting, landing page, sidebar behaviour, translations. | `<space_id>/<collection_id>` |
| [`forumulate_page`](docs/resources/page.md) | A page written in Markdown: place in the tree, publish state, metadata, SEO. | `<space_id>/<page_id>` |
| [`forumulate_forum_category`](docs/resources/forum_category.md) | A forum category: look, moderation, tags, defaults, translations. | `<space_id>/<category_id>` |
| [`forumulate_domain`](docs/resources/domain.md) | A custom hostname for a space or a site, with its DNS records, redirect and IP allow list. | `space/<space_id>/<id>` or `site/<site_id>/<id>` |
| [`forumulate_webhook`](docs/resources/webhook.md) | A signed webhook: events, scope, filters, alert contacts. The signing secret is kept sensitive. | `<webhook_id>` |
| [`forumulate_site`](docs/resources/site.md) | A site: member spaces, shared chrome and theme, page mounts. | `<site slug or id>` |
| [`forumulate_translation_override`](docs/resources/translation_override.md) | One reworded interface string in one language. | `<locale>/<key>` |
| [`forumulate_org_setting`](docs/resources/org_setting.md) | One organization setting from the registry. | `<key>` |
| [`forumulate_connector`](docs/resources/connector.md) | A chat or AI product connected to a space, with an optional knowledge token. | `<space_id>/<connector_id>` |
| [`forumulate_service_connector`](docs/resources/service_connector.md) | An organization-level connector (e-signature, meetings, wallet passes, compliance). | `<provider>` |
| [`forumulate_site_manifest`](docs/resources/site_manifest.md) | The whole site as one `version: 2` manifest, planned by the API's dry run. | `<org id>` |
| [`forumulate_status_component`](docs/resources/status_component.md) | A status page component: any probe kind, group, display, thresholds, regions. | `<space_id>/<component_id>` |
| [`forumulate_status_groups`](docs/resources/status_groups.md) | A status page's group arrangement. | `<space_id>` |
| [`forumulate_status_incident_template`](docs/resources/status_incident_template.md) | An incident template and the words for each later step. | `<space_id>/<template_id>` |
| [`forumulate_status_metric`](docs/resources/status_metric.md) | A custom metric's settings. | `<space_id>/<key>` |

| Data source | What it reads |
| --- | --- |
| [`forumulate_space`](docs/data-sources/space.md) | A space by slug: its id, type, visibility and settings. |
| [`forumulate_space_types`](docs/data-sources/space_types.md) | The space-type vocabulary: piece and container names, tabs, layout presets. |
| [`forumulate_org`](docs/data-sources/org.md) | The organization the provider is configured for. |
| [`forumulate_status_page`](docs/data-sources/status_page.md) | A public status page's state right now. Needs no token. |

## Examples

A docs site, its pages and its address:

```hcl
resource "forumulate_space" "docs" {
  slug       = "docs"
  name       = "Docs"
  type       = "DOCS"
  visibility = "PUBLIC"
}

resource "forumulate_space_locale" "es" {
  space_id = forumulate_space.docs.id
  code     = "es"
  label    = "Español"
}

resource "forumulate_space_settings" "docs" {
  space_id = forumulate_space.docs.id
  seo_json = jsonencode({ seoTitle = "Acme documentation", siteName = "Acme" })
  redirects = [
    { from = "/start", to = "/docs/getting-started", status = 301 },
  ]
}

resource "forumulate_collection" "guides" {
  space_id = forumulate_space.docs.id
  name     = "Guides"
  nav      = { icon = "book", collapsed = false }
}

resource "forumulate_page" "quickstart" {
  space_id      = forumulate_space.docs.id
  collection_id = forumulate_collection.guides.id
  slug          = "quickstart"
  title         = "Quickstart"
  body          = file("${path.module}/quickstart.md")
  metadata      = { tags = ["intro"], seo_title = "Quickstart | Acme Docs" }
}

resource "forumulate_domain" "docs" {
  space_id = forumulate_space.docs.id
  hostname = "docs.example.com"
  activate = true # verifies the DNS records and switches traffic over; apply again after DNS propagates
}

output "docs_dns" {
  value = {
    target       = forumulate_domain.docs.dns_target
    verification = forumulate_domain.docs.verification_record
  }
}
```

A forum, a webhook that tells you when it changes, and an organization-wide setting:

```hcl
resource "forumulate_space" "community" {
  slug = "community"
  name = "Community"
  type = "FORUM"
}

resource "forumulate_forum_category" "help" {
  space_id    = forumulate_space.community.id
  name        = "Help"
  color       = "0b835b"
  topic_kind  = "question"
  default_tags = ["help"]
}

resource "forumulate_webhook" "publish" {
  url            = "https://hooks.example.com/forumulate"
  events         = ["page.published", "topic.created"]
  alert_contacts = ["ops@example.com"]
}

resource "forumulate_org_setting" "typo_tolerance" {
  key   = "search.typo_tolerance"
  value = "true"
}

resource "forumulate_translation_override" "sign_in" {
  locale = "es"
  key    = "reader.sign_in"
  value  = "Entrar"
}
```

A site over several spaces, with a page mounted at `/pricing`:

```hcl
resource "forumulate_site" "main" {
  slug      = "main"
  name      = "Acme"
  space_ids = [forumulate_space.docs.id, forumulate_space.marketing.id]
  config_json = jsonencode({
    copyright   = "© Acme"
    headerLinks = [{ label = "Docs", href = "/docs" }]
  })
  page_mounts = [
    { path = "/pricing", space_id = forumulate_space.marketing.id, page = "pricing" },
  ]
}
```

Connectors:

```hcl
resource "forumulate_connector" "intercom" {      # a chat product connected to a space
  space_id          = forumulate_space.docs.id
  provider_id       = "intercom"
  config            = { workspaceId = "abc123", accessToken = var.intercom_token } # sensitive, write-only
  embed_enabled     = true
  knowledge_enabled = true
  knowledge_scopes  = ["search", "ask"]
  mint_token        = true
}

resource "forumulate_service_connector" "zoom" {  # an organization-level connector
  provider_name = "zoom"
  values        = { clientId = "AbCdEf123" }
  secret_values = { clientSecret = var.zoom_client_secret }
}
```

The whole site at once, from the file the CLI uses:

```hcl
resource "forumulate_site_manifest" "site" {
  manifest_json = jsonencode(yamldecode(file("${path.module}/forumulate.yaml")))
  prune         = true
}

output "docs_space_id" {
  value = forumulate_site_manifest.site.ids["spaces/docs"]
}
```

`terraform plan` prints what the manifest would do (create, update, delete, field by field) as warnings and fails on a
manifest the API would refuse, naming the field. A page or setting changed in the studio shows as pending changes at the
next refresh and is put back by the next apply. Work only you can do (the DNS records of a new domain) is in `manual_json`.

Status pages (components, groups, incident templates, metrics) are covered in
[`docs/resources/status_component.md`](docs/resources/status_component.md) and its siblings.

## Behaviour worth knowing

- **JSON documents** (`*_json`: a theme, SEO, a site's config, page metadata, settings) are merged into what is stored by the
  API. Only the keys you declare are compared on read, and a key you remove from the configuration is cleared, so another
  tool's settings are left alone. Write them with `jsonencode()`; key names are checked against the API's own list at plan time.
- **Destroy** moves a space or page to the trash, which is reversible. Set `destroy_on_delete = true` to delete for good and
  free the slug. A site is dissolved with its spaces intact. A category the API refuses to delete while it holds topics needs
  `delete_move_topics_to` or `delete_force`. `forumulate_site_manifest` forgets by default; `destroy_prunes` deletes what it created.
- **Secrets** are write-only. A webhook's signing secret is shown by the API once and kept, sensitive, in state (an imported
  webhook has none); a connector's `config` and `secret_values` are never returned, so a changed secret is not detected as drift.
- **Retries.** A create carries an `Idempotency-Key` so a retried request cannot make a second object; a 429 waits for
  `Retry-After`. The site manifest's apply has its own timeout (`timeout_seconds`), and an apply the edge cut at 30 s while
  the work carried on is checked with a re-plan before it is called failed.

## Develop

```sh
go build ./...
go vet ./...
go test ./...                       # CRUD of every resource against a fake of the API; plan-time validators; examples vs schema
make e2e TF=/path/to/terraform      # the whole provider under a real Terraform CLI: apply, empty re-plan, drift, import, destroy
make docs                           # regenerate docs/ (needs terraform on PATH); CI fails when docs/ is stale
```

The unit tests run each resource's create, read, update, delete and import through the framework's own interfaces against an
`httptest` fake and need no Terraform binary. `internal/e2e` builds the provider and runs it under a real `terraform`
against a stateful fake, which is what proves Terraform's own consistency rules hold (a value planned is the value applied, and a second
plan after an apply is empty). It is skipped unless `FORUMULATE_E2E_TERRAFORM` names a terraform binary.

## Release

Tag `vX.Y.Z` on `main` and push the tag. `.github/workflows/release.yml` runs the tests, then GoReleaser builds a zip per
platform, a `SHA256SUMS` file, signs it with the GPG key in the repository secrets (`GPG_PRIVATE_KEY`, `PASSPHRASE`) and
attaches everything, plus `terraform-registry-manifest.json` (plugin protocol 6), to the GitHub release. The Terraform
Registry publishes from there once the repository is connected to the `forumulate` namespace and the key's public half is
registered on it; the registry account and the key are the owner's to set up. See [`CHANGELOG.md`](CHANGELOG.md).

## License

MIT, see [`LICENSE`](LICENSE).

---

## About this repository

This repository is a public mirror, refreshed on each Forumulate release
(one commit per release). Pull requests are welcome; accepted changes are
applied upstream and arrive here with the next release.

- API reference: https://forumulate.com/reference
- Guides: https://forumulate.com/guides
- Community and support: https://forumulate.com/community
- Source: https://github.com/forumulate/terraform-provider-forumulate
