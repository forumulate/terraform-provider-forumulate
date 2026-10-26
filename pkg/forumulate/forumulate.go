// Package forumulate exports the provider constructor for programs that embed
// it — the Pulumi bridge (integrations/pulumi-forumulate) builds its provider
// from exactly this, so Terraform and Pulumi cannot drift. The implementation
// stays in internal/ (Go forbids importing internal/ from another module);
// this file is the one public door and must keep tracking provider.New.
package forumulate

import (
	"github.com/forumulate/terraform-provider-forumulate/internal/provider"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

// New returns a factory for the Forumulate provider at the given version.
func New(version string) func() fwprovider.Provider {
	return provider.New(version)
}
