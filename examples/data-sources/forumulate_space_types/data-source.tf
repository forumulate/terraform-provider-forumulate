data "forumulate_space_types" "all" {}

output "space_types" {
  value = data.forumulate_space_types.all.names
}

# What layouts a blog accepts.
output "blog_presets" {
  value = [for t in data.forumulate_space_types.all.types : t.presets if t.type == "BLOG"][0]
}
