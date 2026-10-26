// Command terraform-provider-forumulate is the Terraform provider for
// Forumulate. It manages an organization as code — spaces and their pages,
// collections, forum categories, sites, domains, webhooks, settings, connectors
// and status pages, or the whole site at once as a manifest — through the same
// public API the console uses, authenticated with a personal access token.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/forumulate/terraform-provider-forumulate/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Format the examples, then generate docs/ (the Terraform Registry layout) from
// the provider's schema, the examples/ folder and templates/. It needs a
// terraform binary on PATH.
//go:generate terraform fmt -recursive ./examples/
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.20.1 generate --provider-name forumulate

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// What customers write in required_providers: `source = "forumulate/forumulate"`.
		Address: "registry.terraform.io/forumulate/forumulate",
		Debug:   debug,
	}
	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
