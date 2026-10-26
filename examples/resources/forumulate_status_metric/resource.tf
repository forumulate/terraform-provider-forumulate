resource "forumulate_status_metric" "error_rate" {
  space_id  = var.status_space_id
  key       = "error-rate"
  name      = "Error rate"
  unit      = "%"
  decimals  = 2
  threshold = 1
}
