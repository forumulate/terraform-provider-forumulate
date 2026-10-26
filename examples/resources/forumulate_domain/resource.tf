resource "forumulate_domain" "docs" {
  space_id          = forumulate_space.docs.id
  hostname          = "docs.example.com"
  also_redirect_www = false

  # Set once the DNS records below exist; the apply that follows verifies the
  # name and switches traffic over (apply again after DNS propagates).
  activate = true
}

# Publish the records with your DNS provider.
output "docs_dns_target" {
  value = forumulate_domain.docs.dns_target
}

output "docs_dns_verification" {
  value = forumulate_domain.docs.verification_record
}

# A whole site behind one name.
resource "forumulate_domain" "site" {
  site_id  = forumulate_site.main.id
  hostname = "www.example.com"
}

# Retire an old name by redirecting it.
resource "forumulate_domain" "legacy" {
  space_id    = forumulate_space.docs.id
  hostname    = "old-docs.example.com"
  redirect_to = "https://docs.example.com"
}

# Docs only reachable from the office network.
resource "forumulate_domain" "internal" {
  space_id      = forumulate_space.internal.id
  hostname      = "internal.example.com"
  ip_allow_list = ["203.0.113.0/24"]
}
