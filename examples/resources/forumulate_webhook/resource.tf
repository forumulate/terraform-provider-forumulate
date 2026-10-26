resource "forumulate_webhook" "publish" {
  url    = "https://hooks.example.com/forumulate"
  events = ["page.published", "page.deleted"]

  # Only this space.
  space_id = forumulate_space.docs.id

  alert_contacts = ["ops@example.com"]
}

# The signing secret is shown by the API once and kept, sensitive, in state.
output "publish_webhook_secret" {
  value     = forumulate_webhook.publish.secret
  sensitive = true
}

# A Slack incoming webhook.
resource "forumulate_webhook" "slack" {
  url    = "https://hooks.slack.com/services/T000/B000/XXXX"
  format = "slack"
  events = ["topic.created"]
}
