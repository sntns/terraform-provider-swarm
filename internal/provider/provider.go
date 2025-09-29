package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sntns/terraform-provider-swarm/internal/resources"
)

var (
	_ provider.Provider = &swarmProvider{}
)

// New returns a new provider.
func New() provider.Provider {
	return &swarmProvider{}
}

// swarmProvider is the provider implementation.
type swarmProvider struct{}

// swarmProviderModel maps provider schema data to a Go type.

// Metadata returns the provider type name.
func (p *swarmProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "swarm"
}

// Schema defines the provider-level schema for configuration data.
func (p *swarmProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Swarm provider allows you to manage Docker Swarm clusters.",
		Attributes:  map[string]schema.Attribute{},
	}
}

// Configure prepares a Docker client for data sources and resources.
func (p *swarmProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// No provider-level configuration
}

// DataSources defines the data sources implemented in the provider.
func (p *swarmProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// Resources defines the resources implemented in the provider.
func (p *swarmProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewSwarmInitResource,
		resources.NewSwarmJoinResource,
	}
}

// NodeConfig represents configuration for a specific node
type NodeConfig struct {
	Host                     types.String `tfsdk:"host"`
	CertPath                 types.String `tfsdk:"cert_path"`
	KeyPath                  types.String `tfsdk:"key_path"`
	CaPath                   types.String `tfsdk:"ca_path"`
	APIVersion               types.String `tfsdk:"api_version"`
	Context                  types.String `tfsdk:"context"`
	SSHOpts                  types.List   `tfsdk:"ssh_opts"`
	CaMaterial               types.String `tfsdk:"ca_material"`
	CertMaterial             types.String `tfsdk:"cert_material"`
	KeyMaterial              types.String `tfsdk:"key_material"`
	RegistryAuth             types.Map    `tfsdk:"registry_auth"`
	DisableDockerDaemonCheck types.Bool   `tfsdk:"disable_docker_daemon_check"`
}
