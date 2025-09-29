package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tfTypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/sntns/terraform-provider-swarm/internal/docker"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &swarmJoinResource{}
	_ resource.ResourceWithConfigure = &swarmJoinResource{}
)

// NewSwarmJoinResource is a helper function to simplify the provider implementation.
func NewSwarmJoinResource() resource.Resource {
	return &swarmJoinResource{}
}

// swarmJoinResource is the resource implementation.
type swarmJoinResource struct {
	client *client.Client
}

// swarmJoinResourceModel maps the resource schema data.
type swarmJoinResourceModel struct {
	ID            tfTypes.String `tfsdk:"id"`
	JoinToken     tfTypes.String `tfsdk:"join_token"`
	RemoteAddrs   tfTypes.Set    `tfsdk:"remote_addrs"`
	AdvertiseAddr tfTypes.String `tfsdk:"advertise_addr"`
	ListenAddr    tfTypes.String `tfsdk:"listen_addr"`
	NodeID        tfTypes.String `tfsdk:"node_id"`
	NodeRole      tfTypes.String `tfsdk:"node_role"`
	Node          *docker.TfNode `tfsdk:"node"`
}

// Use the same struct as docker.TfNode for plan.Node

// Metadata returns the resource type name.
func (r *swarmJoinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_join"
}

// Schema defines the schema for the resource.
func (r *swarmJoinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Join a node to an existing Docker Swarm cluster.",
		Attributes: map[string]schema.Attribute{
			"node": docker.NodeSchema,
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
				ElementType: tfTypes.StringType,
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

// Plus de configuration à faire côté provider
func (r *swarmJoinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
}

// Create creates the resource and sets the initial Terraform state.
func (r *swarmJoinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan struct {
		Node          docker.TfNode  `tfsdk:"node"`
		JoinToken     tfTypes.String `tfsdk:"join_token"`
		RemoteAddrs   tfTypes.Set    `tfsdk:"remote_addrs"`
		AdvertiseAddr tfTypes.String `tfsdk:"advertise_addr"`
		ListenAddr    tfTypes.String `tfsdk:"listen_addr"`
		ID            tfTypes.String `tfsdk:"id"`
		NodeID        tfTypes.String `tfsdk:"node_id"`
		NodeRole      tfTypes.String `tfsdk:"node_role"`
	}
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Use shared extraction logic
	dockerConfig := docker.ExtractConfig(plan.Node)
	tflog.Debug(ctx, "Docker client config", map[string]interface{}{
		"host":     dockerConfig.Host,
		"ssh_opts": dockerConfig.SSHOpts,
	})
	dockerClient, err := dockerConfig.NewClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Docker Client",
			"An unexpected error occurred when creating the Docker client. \n\nDocker Client Error: "+err.Error(),
		)
		return
	}
	r.client = dockerClient

	// Get remote addresses
	var remoteAddrs []string
	diags = plan.RemoteAddrs.ElementsAs(ctx, &remoteAddrs, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare join request
	joinRequest := swarm.JoinRequest{
		JoinToken:   plan.JoinToken.ValueString(),
		RemoteAddrs: remoteAddrs,
	}

	if !plan.AdvertiseAddr.IsNull() {
		joinRequest.AdvertiseAddr = plan.AdvertiseAddr.ValueString()
	}

	if !plan.ListenAddr.IsNull() {
		joinRequest.ListenAddr = plan.ListenAddr.ValueString()
	}

	// Join the swarm using Docker API
	err = r.client.SwarmJoin(ctx, joinRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error joining swarm",
			"Could not join swarm, unexpected error: "+err.Error(),
		)
		return
	}

	tflog.Trace(ctx, "joined swarm")

	// Get node info to populate computed fields
	nodeInfo, err := r.client.Info(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting node info",
			"Could not get node info after joining swarm, unexpected error: "+err.Error(),
		)
		return
	}

	nodeID := nodeInfo.Swarm.NodeID
	clusterID := nodeInfo.Swarm.Cluster.ID

	// Determine node role from join token or node info
	nodeRole := "worker"
	if strings.Contains(plan.JoinToken.ValueString(), "SWMTKN-1-") {
		// This is a simplified check - manager tokens typically have different patterns
		parts := strings.Split(plan.JoinToken.ValueString(), "-")
		if len(parts) >= 4 && len(parts[3]) > 30 {
			nodeRole = "manager"
		}
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = tfTypes.StringValue(fmt.Sprintf("%s-%s", nodeID, clusterID))
	plan.NodeID = tfTypes.StringValue(nodeID)
	plan.NodeRole = tfTypes.StringValue(nodeRole)

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *swarmJoinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state swarmJoinResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Recreate Docker client from state.Node if needed
	if r.client == nil {
		if state.Node != nil {
			dockerConfig := docker.ExtractConfig(*state.Node)
			dockerClient, err := dockerConfig.NewClient()
			if err != nil {
				resp.Diagnostics.AddError(
					"Unable to Create Docker Client in Read",
					"An unexpected error occurred when creating the Docker client in Read. \n\nDocker Client Error: "+err.Error(),
				)
				return
			}
			r.client = dockerClient
		}
	}

	// Check if node is still part of swarm
	nodeInfo, err := r.client.Info(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Node Info",
			"Could not read node info: "+err.Error(),
		)
		return
	}

	nodeID := nodeInfo.Swarm.NodeID
	if nodeID == "" {
		// Node is no longer part of swarm, remove from state
		resp.State.RemoveResource(ctx)
		return
	}
	if !state.NodeID.IsNull() && state.NodeID.ValueString() != nodeID {
		resp.State.RemoveResource(ctx)
		resp.Diagnostics.AddWarning(
			"Node ID Changed",
			"The node ID has changed since the last apply. The resource will be recreated.",
		)
		return
	}

	// Update state with current node info
	state.NodeID = tfTypes.StringValue(nodeID)

	// Try to get more detailed node info to determine role
	nodeList, err := r.client.NodeList(ctx, types.NodeListOptions{})
	if err == nil {
		for _, node := range nodeList {
			if node.ID == nodeID {
				if node.Spec.Role == swarm.NodeRoleManager {
					state.NodeRole = tfTypes.StringValue("manager")
				} else {
					state.NodeRole = tfTypes.StringValue("worker")
				}
				break
			}
		}
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *swarmJoinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Most swarm join settings cannot be updated after joining
	resp.Diagnostics.AddError(
		"Swarm Join Update Not Supported",
		"Swarm join settings cannot be updated after joining. Consider recreating the resource with new settings.",
	)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *swarmJoinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state swarmJoinResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Leave the swarm using Docker API
	err := r.client.SwarmLeave(ctx, false) // try regular leave first
	if err != nil {
		// Try force leave if regular leave fails
		err = r.client.SwarmLeave(ctx, true)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Leaving Swarm",
				"Could not leave swarm, unexpected error: "+err.Error(),
			)
			return
		}
	}

	tflog.Trace(ctx, "left swarm")
}
