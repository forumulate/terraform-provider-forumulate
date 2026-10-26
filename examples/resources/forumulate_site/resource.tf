resource "forumulate_site" "main" {
  slug      = "main"
  name      = "Acme"
  space_ids = [forumulate_space.docs.id, forumulate_space.blog.id]

  config_json = jsonencode({
    copyright = "© Acme"
    headerLinks = [
      { label = "Docs", href = "/docs" },
      { label = "Blog", href = "/blog" },
    ]
    cta = { label = "Start free", href = "https://acme.example/signup" }
  })

  # One page of a member space answers at a path of the site.
  page_mounts = [
    { path = "/", space_id = forumulate_space.marketing.id, page = "home" },
    { path = "/pricing", space_id = forumulate_space.marketing.id, page = "pricing" },
  ]
}
