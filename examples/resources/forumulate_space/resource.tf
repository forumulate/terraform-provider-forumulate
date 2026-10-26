resource "forumulate_space" "docs" {
  slug       = "docs"
  name       = "Docs"
  type       = "DOCS"
  visibility = "PUBLIC"

  translations_json = jsonencode({
    es = { name = "Documentación", description = "Guías y referencia" }
  })

  settings_json = jsonencode({
    trashRetentionDays = 30
  })
}

resource "forumulate_space" "community" {
  slug       = "community"
  name       = "Community"
  type       = "FORUM"
  visibility = "PUBLIC"
}

# A space type that has no opinion of its own is shaped at creation.
resource "forumulate_space" "recipes" {
  slug = "recipes"
  name = "Recipes"
  type = "CUSTOM"
  shape = {
    index       = "list"
    order       = "chronological"
    nav         = "tags"
    who_creates = "members"
  }
}
