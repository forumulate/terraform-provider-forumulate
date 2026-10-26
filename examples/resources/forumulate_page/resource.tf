resource "forumulate_page" "quickstart" {
  space_id      = forumulate_space.docs.id
  collection_id = forumulate_collection.guides.id
  slug          = "quickstart"
  title         = "Quickstart"

  body = <<-MD
    Create a space, then publish your first page.

    ```sh
    forumulate publish ./docs
    ```
  MD

  metadata = {
    tags            = ["intro"]
    seo_title       = "Quickstart | Acme Docs"
    seo_description = "Publish your first page in five minutes."
    nav_title       = "Start"
  }

  metadata_json = jsonencode({
    layout = "wide"
  })
}

# A draft: written, not published.
resource "forumulate_page" "wip" {
  space_id      = forumulate_space.docs.id
  collection_id = forumulate_collection.guides.id
  slug          = "wip"
  title         = "Work in progress"
  body          = file("${path.module}/wip.md")
  published     = false
}

# Nested under another page, placed after a sibling.
resource "forumulate_page" "install" {
  space_id      = forumulate_space.docs.id
  collection_id = forumulate_collection.guides.id
  slug          = "install"
  title         = "Install"
  parent_id     = forumulate_page.quickstart.id
  body          = "Install the CLI with `npm i -g @forumulate/cli`."
}
