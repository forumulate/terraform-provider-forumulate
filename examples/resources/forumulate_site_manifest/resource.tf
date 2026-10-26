# The whole site as one document, planned and applied through the API's own
# manifest dry run. `terraform plan` prints every step the manifest would take.
resource "forumulate_site_manifest" "site" {
  manifest_json = jsonencode({
    version = 2

    spaces = [
      {
        slug       = "docs"
        name       = "Docs"
        type       = "DOCS"
        visibility = "PUBLIC"
        locales    = [{ code = "es", label = "Español" }]
        seo        = { seoTitle = "Acme documentation" }
        redirects  = [{ from = "/start", to = "/docs/getting-started" }]
      },
      { slug = "blog", name = "Blog", type = "BLOG", visibility = "PUBLIC" },
    ]

    pages = [
      {
        space = "docs"
        slug  = "getting-started"
        title = "Getting started"
        body  = "Welcome to **Acme**."
      },
    ]

    sites = [
      { slug = "main", name = "Acme", spaces = ["docs", "blog"] },
    ]

    orgSettings = {
      "search.typo_tolerance" = true
    }
  })

  # Delete what the manifest created and no longer declares.
  prune = true
}

# Or keep the document in YAML beside your Markdown:
#   manifest_json = jsonencode(yamldecode(file("${path.module}/forumulate.yaml")))

# Ids of what it manages, e.g. forumulate_site_manifest.site.ids["spaces/docs"].
output "docs_space_id" {
  value = forumulate_site_manifest.site.ids["spaces/docs"]
}

# DNS records and proxy rules only you can create.
output "manual_steps" {
  value = jsondecode(forumulate_site_manifest.site.manual_json)
}
