data "forumulate_status_page" "page" {
  space_id = var.status_space_id
}

# Refuse to roll out while the page reports an outage.
output "page_is_up" {
  value = data.forumulate_status_page.page.overall == "up"
}
