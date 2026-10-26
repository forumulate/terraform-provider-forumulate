resource "forumulate_forum_category" "general" {
  space_id    = forumulate_space.community.id
  name        = "General"
  description = "Talk about anything."
  color       = "0b835b"
  text_color  = "#ffffff"
  emoji       = "💬"

  default_tags = ["welcome"]
  topic_kind   = "discussion"

  translations = {
    es = { name = "General", description = "Habla de lo que quieras." }
  }
}

resource "forumulate_forum_category" "help" {
  space_id                  = forumulate_space.community.id
  name                      = "Help"
  parent_id                 = forumulate_forum_category.general.id
  topic_kind                = "question"
  require_topic_approval    = true
  default_slow_mode_seconds = 30

  # Destroying a category that holds topics moves them first.
  delete_move_topics_to = forumulate_forum_category.general.id
}
