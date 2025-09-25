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

// Ensure the implementation satisfies the expected interfaces.
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
type swarmProviderModel struct {
	Host       types.String `tfsdk:"host"`
	CertPath   types.String `tfsdk:"cert_path"`
	KeyPath    types.String `tfsdk:"key_path"`
	CaPath     types.String `tfsdk:"ca_path"`
	APIVersion types.String `tfsdk:"api_version"`
	Nodes      types.Map    `tfsdk:"nodes"`
}

// Metadata returns the provider type name.
func (p *swarmProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "swarm"
}

// Schema defines the provider-level schema for configuration data.
func (p *swarmProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Swarm provider allows you to manage Docker Swarm clusters.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "Docker daemon host (e.g., tcp://localhost:2376 or unix:///var/run/docker.sock)",
				Optional:    true,
			},
			"cert_path": schema.StringAttribute{
				Description: "Path to client certificate for TLS authentication",
				Optional:    true,
			},
			"key_path": schema.StringAttribute{
				Description: "Path to client key for TLS authentication",
				Optional:    true,
			},
			"ca_path": schema.StringAttribute{
				Description: "Path to CA certificate for TLS authentication",
				Optional:    true,
			},
			"api_version": schema.StringAttribute{
				Description: "Docker API version",
				Optional:    true,
			},
			"nodes": schema.MapNestedAttribute{
				Description: "Map of node configurations for multi-host cluster management",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"host": schema.StringAttribute{
							Description: "Docker daemon host for this node",
							Required:    true,
						},
						"cert_path": schema.StringAttribute{
							Description: "Path to client certificate for TLS authentication",
							Optional:    true,
						},
						"key_path": schema.StringAttribute{
							Description: "Path to client key for TLS authentication", 
							Optional:    true,
						},
						"ca_path": schema.StringAttribute{
							Description: "Path to CA certificate for TLS authentication",
							Optional:    true,
						},
						"api_version": schema.StringAttribute{
							Description: "Docker API version",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

// Configure prepares a Docker client for data sources and resources.
func (p *swarmProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve provider data from configuration
	var config swarmProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available
	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	host := config.Host.ValueString()
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	certPath := config.CertPath.ValueString()
	keyPath := config.KeyPath.ValueString()
	caPath := config.CaPath.ValueString()
	apiVersion := config.APIVersion.ValueString()

	// Create default Docker client configuration
	defaultClientConfig := &DockerClientConfig{
		Host:       host,
		CertPath:   certPath,
		KeyPath:    keyPath,
		CaPath:     caPath,
		APIVersion: apiVersion,
	}

	// Create multi-node configuration
	nodeConfigs := make(map[string]*DockerClientConfig)
	
	// Parse nodes configuration if provided
	if !config.Nodes.IsNull() {
		var nodesMap map[string]NodeConfig
		diags = config.Nodes.ElementsAs(ctx, &nodesMap, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		for nodeName, nodeConfig := range nodesMap {
			nodeHost := nodeConfig.Host.ValueString()
			if nodeHost == "" {
				nodeHost = "unix:///var/run/docker.sock"
			}

			nodeConfigs[nodeName] = &DockerClientConfig{
				Host:       nodeHost,
				CertPath:   nodeConfig.CertPath.ValueString(),
				KeyPath:    nodeConfig.KeyPath.ValueString(),
				CaPath:     nodeConfig.CaPath.ValueString(),
				APIVersion: nodeConfig.APIVersion.ValueString(),
			}
		}
	}

	// Create comprehensive provider data
	providerData := &SwarmProviderData{
		DefaultConfig: defaultClientConfig,
		NodeConfigs:   nodeConfigs,
	}

	// Make the Docker client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = providerData
	resp.ResourceData = providerData
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

// DockerClientConfig holds the configuration for connecting to Docker daemon
type DockerClientConfig struct {
	Host       string
	CertPath   string
	KeyPath    string
	CaPath     string
	APIVersion string
}

// NodeConfig represents configuration for a specific node
type NodeConfig struct {
	Host       types.String `tfsdk:"host"`
	CertPath   types.String `tfsdk:"cert_path"`
	KeyPath    types.String `tfsdk:"key_path"`
	CaPath     types.String `tfsdk:"ca_path"`
	APIVersion types.String `tfsdk:"api_version"`
}

// SwarmProviderData holds comprehensive provider configuration
type SwarmProviderData struct {
	DefaultConfig *DockerClientConfig
	NodeConfigs   map[string]*DockerClientConfig
}
