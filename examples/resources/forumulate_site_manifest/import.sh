# The manifest adopts what is already there: import by organization id, then set
# manifest_json to the document GET /orgs/{orgId}/manifest?export=true answers.
terraform import forumulate_site_manifest.site 00000000-0000-4000-8000-000000000000
