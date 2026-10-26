data "forumulate_org" "this" {}

output "org_slug" {
  value = data.forumulate_org.this.slug
}
