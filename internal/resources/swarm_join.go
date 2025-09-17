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
	_ resource.Resource              = &swarmJoinSimpleResource{}
	_ resource.ResourceWithConfigure = &swarmJoinSimpleResource{}
)

// NewSwarmJoinResource is a helper function to simplify the provider implementation.
func NewSwarmJoinResource() resource.Resource {
	return &swarmJoinSimpleResource{}
}

// swarmJoinSimpleResource is the resource implementation.
type swarmJoinSimpleResource struct {
	dockerHost string
}

// swarmJoinSimpleResourceModel maps the resource schema data.
type swarmJoinSimpleResourceModel struct {
	ID            types.String `tfsdk:"id"`
	JoinToken     types.String `tfsdk:"join_token"`
	RemoteAddrs   types.Set    `tfsdk:"remote_addrs"`
	AdvertiseAddr types.String `tfsdk:"advertise_addr"`
	ListenAddr    types.String `tfsdk:"listen_addr"`
	NodeID        types.String `tfsdk:"node_id"`
	NodeRole      types.String `tfsdk:"node_role"`
}

// Metadata returns the resource type name.
func (r *swarmJoinSimpleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_join"
}

// Schema defines the schema for the resource.
func (r *swarmJoinSimpleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Join a node to an existing Docker Swarm cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"join_token": schema.StringAttribute{
				Description: "Join token from the swarm manager",
				Required:    true,
				Sensitive:   true,
			},
			"remote_addrs": schema.SetAttribute{
				Description: "Addresses of existing swarm managers",
				Required:    true,
				ElementType: types.StringType,
			},
			"advertise_addr": schema.StringAttribute{
				Description: "Externally reachable address advertised to other nodes",
				Optional:    true,
			},
			"listen_addr": schema.StringAttribute{
				Description: "Listen address for the raft consensus protocol (managers only)",
				Optional:    true,
			},
			"node_id": schema.StringAttribute{
				Description: "ID of the node after joining",
				Computed:    true,
			},
			"node_role": schema.StringAttribute{
				Description: "Role of the node (manager or worker)",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *swarmJoinSimpleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *swarmJoinSimpleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan swarmJoinSimpleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get remote addresses
	var remoteAddrs []string
	diags = plan.RemoteAddrs.ElementsAs(ctx, &remoteAddrs, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build docker swarm join command
	args := []string{"swarm", "join", "--token", plan.JoinToken.ValueString()}

	if !plan.AdvertiseAddr.IsNull() {
		args = append(args, "--advertise-addr", plan.AdvertiseAddr.ValueString())
	}

	if !plan.ListenAddr.IsNull() {
		args = append(args, "--listen-addr", plan.ListenAddr.ValueString())
	}

	// Add remote addresses
	for _, addr := range remoteAddrs {
		args = append(args, addr)
	}

	// Execute docker swarm join
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error joining swarm",
			"Could not join swarm, unexpected error: "+err.Error()+"\nOutput: "+string(output),
		)
		return
	}

	tflog.Trace(ctx, "joined swarm", map[string]interface{}{
		"output": string(output),
	})

	// Get node info
	nodeInfoCmd := exec.Command("docker", "info", "--format", "{{.Swarm.NodeID}}")
	nodeInfoOutput, err := nodeInfoCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting node info",
			"Could not get node info after joining swarm, unexpected error: "+err.Error(),
		)
		return
	}

	nodeID := strings.TrimSpace(string(nodeInfoOutput))

	// Get cluster ID
	clusterInfoCmd := exec.Command("docker", "info", "--format", "{{.Swarm.Cluster.ID}}")
	clusterInfoOutput, err := clusterInfoCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting cluster info",
			"Could not get cluster info after joining swarm, unexpected error: "+err.Error(),
		)
		return
	}

	clusterID := strings.TrimSpace(string(clusterInfoOutput))

	// Determine role based on token (simplified approach)
	nodeRole := "worker"
	if strings.Contains(plan.JoinToken.ValueString(), "SWMTKN-1-") {
		// Check if it's a manager token by its characteristics
		parts := strings.Split(plan.JoinToken.ValueString(), "-")
		if len(parts) >= 4 && len(parts[3]) > 30 {
			nodeRole = "manager"
		}
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = types.StringValue(fmt.Sprintf("%s-%s", nodeID, clusterID))
	plan.NodeID = types.StringValue(nodeID)
	plan.NodeRole = types.StringValue(nodeRole)

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *swarmJoinSimpleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state swarmJoinSimpleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if node is still part of swarm
	nodeInfoCmd := exec.Command("docker", "info", "--format", "{{.Swarm.NodeID}}")
	nodeInfoOutput, err := nodeInfoCmd.Output()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Node Info",
			"Could not read node info: "+err.Error(),
		)
		return
	}

	nodeID := strings.TrimSpace(string(nodeInfoOutput))
	if nodeID == "" {
		// Node is no longer part of swarm, remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current node info
	state.NodeID = types.StringValue(nodeID)

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *swarmJoinSimpleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Most swarm join settings cannot be updated after joining
	resp.Diagnostics.AddError(
		"Swarm Join Update Not Supported",
		"Swarm join settings cannot be updated after joining. Consider recreating the resource with new settings.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *swarmJoinSimpleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Leave the swarm
	cmd := exec.Command("docker", "swarm", "leave")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try force leave if regular leave fails
		forceCmd := exec.Command("docker", "swarm", "leave", "--force")
		forceOutput, forceErr := forceCmd.CombinedOutput()
		if forceErr != nil {
			resp.Diagnostics.AddError(
				"Error Leaving Swarm",
				"Could not leave swarm, unexpected error: "+forceErr.Error()+"\nOutput: "+string(forceOutput),
			)
			return
		}
	}

	tflog.Trace(ctx, "left swarm", map[string]interface{}{
		"output": string(output),
	})
}
