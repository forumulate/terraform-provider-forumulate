data "forumulate_space" "docs" {
  slug = "docs"
}

resource "forumulate_collection" "guides" {
  space_id = data.forumulate_space.docs.id
  name     = "Guides"
}
