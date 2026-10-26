resource "forumulate_status_groups" "page" {
  space_id = var.status_space_id
  group {
    name        = "Core"
    description = "What every customer touches."
  }
  group {
    name        = "Integrations"
    layout      = "grid-2"
    hide_uptime = true
  }
}
