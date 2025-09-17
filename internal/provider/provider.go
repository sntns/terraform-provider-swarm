package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure SwarmProvider satisfies various provider interfaces.
var _ provider.Provider = &SwarmProvider{}
var _ provider.ProviderWithFunctions = &SwarmProvider{}

// SwarmProvider defines the provider implementation.
type SwarmProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// SwarmProviderModel describes the provider data model.
type SwarmProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *SwarmProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "swarm"
	resp.Version = p.version
}

func (p *SwarmProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Docker Swarm API endpoint URL",
				Optional:            true,
			},
		},
	}
}

func (p *SwarmProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SwarmProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	// if data.Endpoint.IsNull() { /* ... */ }

	// Example client configuration for data sources and resources
	// client := http.DefaultClient
	// resp.DataSourceData = client
	// resp.ResourceData = client
}

func (p *SwarmProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServiceResource,
	}
}

func (p *SwarmProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewServiceDataSource,
	}
}

func (p *SwarmProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{
		// Example function
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SwarmProvider{
			version: version,
		}
	}
}