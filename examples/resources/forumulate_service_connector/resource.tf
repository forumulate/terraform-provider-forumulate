variable "zoom_client_secret" {
  type      = string
  sensitive = true
}

resource "forumulate_service_connector" "zoom" {
  provider_name = "zoom"
  enabled       = true

  values = {
    clientId = "AbCdEf123"
  }

  # Write-only: the API reports which secrets are set, never their values.
  secret_values = {
    clientSecret = var.zoom_client_secret
  }
}

output "zoom_callback_url" {
  value = forumulate_service_connector.zoom.callback_url
}
