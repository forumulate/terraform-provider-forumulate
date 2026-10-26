resource "forumulate_status_component" "api" {
  space_id         = var.status_space_id
  name             = "API"
  url              = "https://api.example.com/health"
  group            = "Core"
  regions          = ["eu-west", "us-east"]
  confirm_regions  = 2
  bar_days         = 90
  interval_seconds = 60
}

resource "forumulate_status_component" "database" {
  space_id   = var.status_space_id
  name       = "Database"
  kind       = "tcp"
  group      = "Core"
  probe_json = jsonencode({ host = "db.example.com", port = 5432 })
}

resource "forumulate_status_component" "nightly_backup" {
  space_id         = var.status_space_id
  name             = "Nightly backup"
  kind             = "heartbeat"
  interval_seconds = 90000
  display          = "status"
}

output "backup_ping_url" {
  value = forumulate_status_component.nightly_backup.ping_url
}
