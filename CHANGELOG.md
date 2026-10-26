# Changelog

## 0.2.0

The provider now manages the whole platform, not just status pages.

### New resources

- `forumulate_space` — a space of any type: slug, name, type, visibility, translations, settings, tags. Destroy trashes it; `destroy_on_delete` makes it permanent.
- `forumulate_space_locale` — a language a docs space publishes in.
- `forumulate_space_settings` — a space's layout, SEO, theme, redirect table and its type's own settings (blog, forum, status).
- `forumulate_collection` — nesting, landing page, sidebar behaviour, translations.
- `forumulate_page` — Markdown body, place in the tree, publish state, metadata and SEO; drift in the body is told apart from the API's rewriting of your Markdown.
- `forumulate_forum_category` — look, moderation, tags, defaults and translations.
- `forumulate_domain` — a custom hostname for a space or a site; exposes the DNS records to publish and verifies and activates on request.
- `forumulate_webhook` — events, scope, filters, alert contacts; the signing secret is kept sensitive.
- `forumulate_site` — member spaces, shared chrome and theme, page mounts.
- `forumulate_translation_override`, `forumulate_org_setting` — interface strings and organization settings.
- `forumulate_connector` — a chat or AI product connected to a space, with an optional knowledge token.
- `forumulate_service_connector` — an organization-level connector (e-signature, meetings, wallet passes, compliance).
- `forumulate_site_manifest` — the whole site as one `version: 2` manifest; `terraform plan` prints the API's dry run, `terraform apply` applies it.

### New data sources

- `forumulate_space` (by slug), `forumulate_space_types`, `forumulate_org`. `forumulate_status_page` is unchanged.

### Changed

- Every request pins `Forumulate-Version: 2027-02-01` (a taken slug is a conflict, unknown fields are refused, lists answer under `data`). The status resources read lists under either key.
- API errors are read from the real error envelope (`error.code`, `message`, `param`, `hint`, `details`, request id) and shown on the attribute they name.
- Creates carry an `Idempotency-Key`; a 429 waits for `Retry-After`; gateway errors on reads and keyed creates are retried.
- Plan-time validation of names, slugs, enums, lengths, counts, JSON document keys and webhook events, taken from the API's own schemas.
- Registry documentation in `docs/`, examples for every resource and data source in `examples/`, GoReleaser configuration, and release and test workflows for the public repository.

### Not changed

- The four status resources and their import ids.

## 0.1.0

- `forumulate_status_component`, `forumulate_status_groups`, `forumulate_status_incident_template`, `forumulate_status_metric` and the `forumulate_status_page` data source.
