resource "forumulate_collection" "guides" {
  space_id    = forumulate_space.docs.id
  name        = "Guides"
  slug        = "guides"
  description = "Start here."

  nav = {
    icon      = "book"
    collapsed = false
    hub_cards = true
  }

  seo = {
    title = "Guides"
    index = "index"
  }

  translations = {
    es = { name = "Guías", description = "Empieza aquí." }
  }
}

resource "forumulate_collection" "api" {
  space_id  = forumulate_space.docs.id
  name      = "API"
  parent_id = forumulate_collection.guides.id
}
