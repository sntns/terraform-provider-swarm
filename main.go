// The swarm provider manages Docker Swarm clusters: one resource per
// cluster, which reaches each node at its own Docker.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/sntns/terraform-provider-swarm/internal/provider"
)

//go:generate terraform fmt -recursive ./examples/
//go:generate go tool tfplugindocs generate --provider-name swarm

// version is the release's, which GoReleaser sets.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider for a debugger, such as delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/sntns/swarm",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err)
	}
}
