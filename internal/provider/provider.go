package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/sntns/terraform-provider-swarm/internal/docker"
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
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "Docker daemon host. Defaults to unix:///var/run/docker.sock",
				Optional:    true,
			},
			"cert_path": schema.StringAttribute{
				Description: "Path to directory with Docker TLS config",
				Optional:    true,
			},
			"key_path": schema.StringAttribute{
				Description: "Path to Docker client private key",
				Optional:    true,
			},
			"ca_path": schema.StringAttribute{
				Description: "Path to Docker CA certificate",
				Optional:    true,
			},
			"api_version": schema.StringAttribute{
				Description: "Docker API version to use",
				Optional:    true,
			},
			"registry_auth": schema.MapAttribute{
				Description: "Registry authentication configuration",
				ElementType: types.StringType,
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// Configure prepares a Docker client for data sources and resources.
func (p *swarmProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Swarm provider")
	
	var config resources.SwarmProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set defaults if not specified
	host := config.Host.ValueString()
	if host == "" {
		if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
			host = dockerHost
		} else {
			host = "unix:///var/run/docker.sock"
		}
	}

	// Create Docker client configuration
	dockerConfig := &docker.Config{
		Host: host,
	}

	if !config.CertPath.IsNull() {
		dockerConfig.CertPath = config.CertPath.ValueString()
	}

	// Create the Docker client
	dockerClient, err := dockerConfig.NewClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Docker Client",
			"An unexpected error occurred when creating the Docker client. "+
				"Please verify your Docker configuration and ensure Docker is running.\n\n"+
				"Docker Client Error: "+err.Error(),
		)
		return
	}

	// Test the connection
	_, err = dockerClient.Info(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Docker Connection Warning",
			"Unable to connect to Docker daemon. Some resources may not work correctly.\n\n"+
				"Docker Error: "+err.Error(),
		)
	}

	// Store configuration for use in resources
	providerData := &resources.SwarmProviderData{
		NodeConfigs: map[string]*resources.DockerClientConfig{
			"default": {
				Host:       host,
				CertPath:   config.CertPath.ValueString(),
				APIVersion: config.APIVersion.ValueString(),
			},
		},
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData

	tflog.Info(ctx, "Configured Swarm provider", map[string]any{
		"host": host,
	})
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
