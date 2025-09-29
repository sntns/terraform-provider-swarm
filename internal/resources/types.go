package resources

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DockerClientConfig represents the Docker client configuration
type DockerClientConfig struct {
	Host       string
	CertPath   string
	KeyPath    string
	CaPath     string
	APIVersion string
}

// SwarmProviderData holds comprehensive provider configuration
type SwarmProviderData struct {
	NodeConfigs map[string]*DockerClientConfig
}

// SwarmProviderModel represents the provider configuration schema
type SwarmProviderModel struct {
	Host         types.String `tfsdk:"host"`
	CertPath     types.String `tfsdk:"cert_path"`
	KeyPath      types.String `tfsdk:"key_path"`
	CaPath       types.String `tfsdk:"ca_path"`
	APIVersion   types.String `tfsdk:"api_version"`
	RegistryAuth types.Map    `tfsdk:"registry_auth"`
}
