# Booleans are "true"/"false"; integers are digits; enums are one of their options.
resource "forumulate_org_setting" "typo_tolerance" {
  key   = "search.typo_tolerance"
  value = "true"
}

resource "forumulate_org_setting" "trash" {
  key   = "trash.retention_days"
  value = "60"
}
