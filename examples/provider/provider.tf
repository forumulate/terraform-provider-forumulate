terraform {
  required_providers {
    forumulate = {
      source = "forumulate/forumulate"
    }
  }
}

# Reads FORUMULATE_TOKEN (a personal access token), FORUMULATE_ORG_ID and,
# optionally, FORUMULATE_API_URL from the environment. Prefer that to writing
# the token here, so it never lands in configuration or version control.
provider "forumulate" {}
