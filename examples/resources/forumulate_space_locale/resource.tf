resource "forumulate_space_locale" "es" {
  space_id = forumulate_space.docs.id
  code     = "es"
  label    = "Español"
}

# The source language is only relabelled.
resource "forumulate_space_locale" "source" {
  space_id = forumulate_space.docs.id
  code     = "default"
  label    = "English"
}
