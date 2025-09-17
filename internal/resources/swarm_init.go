package resources

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &swarmInitSimpleResource{}
	_ resource.ResourceWithConfigure = &swarmInitSimpleResource{}
)

// NewSwarmInitResource is a helper function to simplify the provider implementation.
func NewSwarmInitResource() resource.Resource {
	return &swarmInitSimpleResource{}
}

// swarmInitSimpleResource is the resource implementation.
type swarmInitSimpleResource struct {
	dockerHost string
}

// swarmInitSimpleResourceModel maps the resource schema data.
type swarmInitSimpleResourceModel struct {
	ID            types.String `tfsdk:"id"`
	AdvertiseAddr types.String `tfsdk:"advertise_addr"`
	ListenAddr    types.String `tfsdk:"listen_addr"`
	ManagerToken  types.String `tfsdk:"manager_token"`
	WorkerToken   types.String `tfsdk:"worker_token"`
}

// Metadata returns the resource type name.
func (r *swarmInitSimpleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_init"
}

// Schema defines the schema for the resource.
func (r *swarmInitSimpleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
func (r *swarmInitSimpleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.dockerHost = clientConfig.Host
}

// Create creates the resource and sets the initial Terraform state.
func (r *swarmInitSimpleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan swarmInitSimpleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build docker swarm init command
	args := []string{"swarm", "init"}

	if !plan.AdvertiseAddr.IsNull() {
		args = append(args, "--advertise-addr", plan.AdvertiseAddr.ValueString())
	}

	if !plan.ListenAddr.IsNull() {
		args = append(args, "--listen-addr", plan.ListenAddr.ValueString())
	}

	// Execute docker swarm init
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error initializing swarm",
			"Could not initialize swarm, unexpected error: "+err.Error()+"\nOutput: "+string(output),
		)
		return
	}

	tflog.Trace(ctx, "initialized swarm", map[string]interface{}{
		"output": string(output),
	})

	// Get swarm info
	infoCmd := exec.Command("docker", "info", "--format", "{{.Swarm.Cluster.ID}}")
	infoOutput, err := infoCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting swarm info",
			"Could not get swarm info after initialization, unexpected error: "+err.Error(),
		)
		return
	}

	swarmID := strings.TrimSpace(string(infoOutput))

	// Get manager join token
	managerTokenCmd := exec.Command("docker", "swarm", "join-token", "manager", "-q")
	managerTokenOutput, err := managerTokenCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting manager join token",
			"Could not get manager join token, unexpected error: "+err.Error(),
		)
		return
	}

	// Get worker join token
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
	plan.ID = types.StringValue(swarmID)
	plan.ManagerToken = types.StringValue(strings.TrimSpace(string(managerTokenOutput)))
	plan.WorkerToken = types.StringValue(strings.TrimSpace(string(workerTokenOutput)))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *swarmInitSimpleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state swarmInitSimpleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if swarm is still active
	infoCmd := exec.Command("docker", "info", "--format", "{{.Swarm.Cluster.ID}}")
	infoOutput, err := infoCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Swarm",
			"Could not read swarm ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	swarmID := strings.TrimSpace(string(infoOutput))
	if swarmID == "" {
		// Swarm is no longer active, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current swarm info
	state.ID = types.StringValue(swarmID)

	// Get current join tokens
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

	state.ManagerToken = types.StringValue(strings.TrimSpace(string(managerTokenOutput)))
	state.WorkerToken = types.StringValue(strings.TrimSpace(string(workerTokenOutput)))

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *swarmInitSimpleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Most swarm init settings cannot be updated after creation
	resp.Diagnostics.AddError(
		"Swarm Init Update Not Supported",
		"Swarm initialization settings cannot be updated after creation. Consider recreating the resource.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *swarmInitSimpleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Leave the swarm
	cmd := exec.Command("docker", "swarm", "leave", "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Swarm",
			"Could not delete swarm, unexpected error: "+err.Error()+"\nOutput: "+string(output),
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
