resource "forumulate_space_settings" "docs" {
  space_id = forumulate_space.docs.id

  preset = "standard"

  seo_json = jsonencode({
    seoTitle       = "Acme documentation"
    description    = "How to build with Acme."
    siteName       = "Acme"
    titleTemplate  = "{title} | Acme Docs"
    socialImageUrl = "https://acme.example/og.png"
  })

  theme_json = jsonencode({
    primaryColor = "#0b835b"
    accentColor  = "#f59e0b"
  })

  redirects = [
    { from = "/docs/start", to = "/docs/getting-started", status = 301 },
    { from = "/old/*", to = "/new/*" },
  ]
}

# A blog's own settings live in the same resource.
resource "forumulate_space_settings" "blog" {
  space_id = forumulate_space.blog.id

  blog_settings_json = jsonencode({
    whoCanPost       = "members"
    moderateComments = true
    commentOrder     = "newest"
  })
}
