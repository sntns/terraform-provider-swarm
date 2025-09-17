package resources

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tfTypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &swarmInitResource{}
	_ resource.ResourceWithConfigure = &swarmInitResource{}
)

// NewSwarmInitResource is a helper function to simplify the provider implementation.
func NewSwarmInitResource() resource.Resource {
	return &swarmInitResource{}
}

// swarmInitResource is the resource implementation.
type swarmInitResource struct {
	client *client.Client
}

// swarmInitResourceModel maps the resource schema data.
type swarmInitResourceModel struct {
	ID            tfTypes.String `tfsdk:"id"`
	AdvertiseAddr tfTypes.String `tfsdk:"advertise_addr"`
	ListenAddr    tfTypes.String `tfsdk:"listen_addr"`
	ManagerToken  tfTypes.String `tfsdk:"manager_token"`
	WorkerToken   tfTypes.String `tfsdk:"worker_token"`
}

// Metadata returns the resource type name.
func (r *swarmInitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_init"
}

// Schema defines the schema for the resource.
func (r *swarmInitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Initialize a Docker Swarm cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Swarm cluster ID",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"advertise_addr": schema.StringAttribute{
				Description: "Externally reachable address advertised to other nodes",
				Optional:    true,
			},
			"listen_addr": schema.StringAttribute{
				Description: "Listen address for the raft consensus protocol",
				Optional:    true,
			},
			"manager_token": schema.StringAttribute{
				Description: "Token for joining as a manager",
				Computed:    true,
				Sensitive:   true,
			},
			"worker_token": schema.StringAttribute{
				Description: "Token for joining as a worker",
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *swarmInitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	clientConfig, ok := req.ProviderData.(*DockerClientConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *DockerClientConfig, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	// Create Docker client
	var httpClient *http.Client
	if clientConfig.CertPath != "" && clientConfig.KeyPath != "" && clientConfig.CaPath != "" {
		tlsConfig := &tls.Config{}
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
	}

	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(clientConfig.Host),
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Docker Client",
			"An unexpected error occurred when creating the Docker client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Docker Client Error: "+err.Error(),
		)
		return
	}

	r.client = dockerClient
}

// Create creates the resource and sets the initial Terraform state.
func (r *swarmInitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan swarmInitResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Initialize swarm
	initRequest := swarm.InitRequest{}
	
	if !plan.AdvertiseAddr.IsNull() {
		initRequest.AdvertiseAddr = plan.AdvertiseAddr.ValueString()
	}
	
	if !plan.ListenAddr.IsNull() {
		initRequest.ListenAddr = plan.ListenAddr.ValueString()
	}

	// Execute docker swarm init using API
	nodeID, err := r.client.SwarmInit(ctx, initRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error initializing swarm",
			"Could not initialize swarm, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "initialized swarm", map[string]interface{}{
		"node_id": nodeID,
	})

	// Get swarm info to retrieve cluster ID
	swarmInfo, err := r.client.SwarmInspect(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error inspecting swarm",
			"Could not inspect swarm after initialization, unexpected error: "+err.Error(),
		)
		return
	}

	// Get join tokens using Docker CLI (hybrid approach)
	// Docker API doesn't expose join tokens directly, so we use CLI for this specific operation
	managerTokenCmd := exec.Command("docker", "swarm", "join-token", "manager", "-q")
	managerTokenOutput, err := managerTokenCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting manager join token",
			"Could not get manager join token, unexpected error: "+err.Error(),
		)
		return
	}

	workerTokenCmd := exec.Command("docker", "swarm", "join-token", "worker", "-q")
	workerTokenOutput, err := workerTokenCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting worker join token",  
			"Could not get worker join token, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = tfTypes.StringValue(swarmInfo.ID)
	plan.ManagerToken = tfTypes.StringValue(strings.TrimSpace(string(managerTokenOutput)))
	plan.WorkerToken = tfTypes.StringValue(strings.TrimSpace(string(workerTokenOutput)))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *swarmInitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state swarmInitResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get current swarm info
	swarmInfo, err := r.client.SwarmInspect(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Swarm",
			"Could not read swarm ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Check if we got valid swarm info
	if swarmInfo.ID == "" {
		// Swarm is no longer active, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current swarm info
	state.ID = tfTypes.StringValue(swarmInfo.ID)

	// Get current join tokens using Docker CLI
	managerTokenCmd := exec.Command("docker", "swarm", "join-token", "manager", "-q")
	managerTokenOutput, err := managerTokenCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting manager join token",
			"Could not get manager join token: "+err.Error(),
		)
		return
	}

	workerTokenCmd := exec.Command("docker", "swarm", "join-token", "worker", "-q")
	workerTokenOutput, err := workerTokenCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting worker join token",
			"Could not get worker join token: "+err.Error(),
		)
		return
	}

	state.ManagerToken = tfTypes.StringValue(strings.TrimSpace(string(managerTokenOutput)))
	state.WorkerToken = tfTypes.StringValue(strings.TrimSpace(string(workerTokenOutput)))

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *swarmInitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Most swarm init settings cannot be updated after creation
	resp.Diagnostics.AddError(
		"Swarm Init Update Not Supported",
		"Swarm initialization settings cannot be updated after creation. Consider recreating the resource.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *swarmInitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state swarmInitResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Leave the swarm (force leave to ensure it works)
	err := r.client.SwarmLeave(ctx, true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Swarm",
			"Could not delete swarm, unexpected error: "+err.Error(),
		)
		return
	}
}

// DockerClientConfig represents the Docker client configuration
type DockerClientConfig struct {
	Host       string
	CertPath   string
	KeyPath    string
	CaPath     string
	APIVersion string
}
