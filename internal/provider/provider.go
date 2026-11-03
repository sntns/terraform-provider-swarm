// Package provider is the swarm provider: a swarm_cluster resource that
// inits or adopts a Docker Swarm cluster and keeps its nodes as the
// configuration gives them.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &swarmProvider{}

type swarmProvider struct {
	version string
}

// New returns the provider of the version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &swarmProvider{version: version}
	}
}

func (p *swarmProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName, resp.Version = "swarm", p.version
}

func (p *swarmProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Docker Swarm clusters. A `swarm_cluster` inits its cluster, or adopts one that runs already, " +
			"and keeps its nodes as the configuration gives them. Each node gives its own Docker endpoint, " +
			"so the provider takes no configuration and one configuration can manage several clusters.",
	}
}

func (p *swarmProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *swarmProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewSwarmClusterResource}
}

func (p *swarmProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
