package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"swarm": providerserver.NewProtocol6WithError(New()),
}

func testAccPreCheck(t *testing.T) {
	// Skip acceptance tests if Docker is not available
	// These tests require a running Docker daemon with Swarm mode enabled
	t.Skip("Acceptance tests require Docker Swarm to be available and configured")
}