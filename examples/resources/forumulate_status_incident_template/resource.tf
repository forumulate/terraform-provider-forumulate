resource "forumulate_status_incident_template" "db_failover" {
  space_id        = var.status_space_id
  name            = "Database failover"
  title           = "Database failover in progress"
  severity        = "major"
  markdown        = "We are failing over **{{region}}** to its replica."
  monitoring_text = "The replica is serving traffic; we are watching it."
  resolved_text   = "The failover is complete."
}
