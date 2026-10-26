variable "intercom_token" {
  type      = string
  sensitive = true
}

# Connect a chat product to a docs space: embed its widget on the reader and
# let it read the space's knowledge base.
resource "forumulate_connector" "intercom" {
  space_id    = forumulate_space.docs.id
  provider_id = "intercom"

  # Sensitive and write-only: the API reports which fields are set, not their values.
  config = {
    workspaceId = "abc123"
    accessToken = var.intercom_token
  }

  embed_enabled     = true
  knowledge_enabled = true
  knowledge_scopes  = ["search", "ask", "llms"]

  appearance = {
    primary_color = "#0b835b"
    color_scheme  = "system"
    position      = "right"
    greeting      = "Questions? Ask us."
  }

  # An access token for the product to read the knowledge base with, shown once.
  mint_token = true
}

output "intercom_knowledge_token" {
  value     = forumulate_connector.intercom.token
  sensitive = true
}

output "intercom_missing_fields" {
  value = forumulate_connector.intercom.missing_fields
}
