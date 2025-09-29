package resources

import (
	"context"

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
	ID            tfTypes.String      `tfsdk:"id"`
	AdvertiseAddr tfTypes.String      `tfsdk:"advertise_addr"`
	ListenAddr    tfTypes.String      `tfsdk:"listen_addr"`
	ManagerToken  tfTypes.String      `tfsdk:"manager_token"`
	WorkerToken   tfTypes.String      `tfsdk:"worker_token"`
	Node          *swarmInitNodeModel `tfsdk:"node"`
}

type swarmInitNodeModel struct {
	Host         tfTypes.String `tfsdk:"host"`
	Context      tfTypes.String `tfsdk:"context"`
	SSHOpts      tfTypes.List   `tfsdk:"ssh_opts"`
	CertMaterial tfTypes.String `tfsdk:"cert_material"`
	KeyMaterial  tfTypes.String `tfsdk:"key_material"`
	CaMaterial   tfTypes.String `tfsdk:"ca_material"`
	CertPath     tfTypes.String `tfsdk:"cert_path"`
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
			"node": docker.NodeSchema,
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

// Plus de configuration à faire côté provider
func (r *swarmInitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
}

// Create creates the resource and sets the initial Terraform state.
func (r *swarmInitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan struct {
		Node          docker.TfNode  `tfsdk:"node"`
		AdvertiseAddr tfTypes.String `tfsdk:"advertise_addr"`
		ListenAddr    tfTypes.String `tfsdk:"listen_addr"`
		ID            tfTypes.String `tfsdk:"id"`
		ManagerToken  tfTypes.String `tfsdk:"manager_token"`
		WorkerToken   tfTypes.String `tfsdk:"worker_token"`
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

	// Initialize swarm
	initRequest := swarm.InitRequest{}
	if !plan.AdvertiseAddr.IsNull() {
		initRequest.AdvertiseAddr = plan.AdvertiseAddr.ValueString()
	}
	if !plan.ListenAddr.IsNull() {
		initRequest.ListenAddr = plan.ListenAddr.ValueString()
	}
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
	swarmInfo, err := r.client.SwarmInspect(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error inspecting swarm",
			"Could not inspect swarm after initialization, unexpected error: "+err.Error(),
		)
		return
	}
	swarmInfoWithTokens, err := r.client.SwarmInspect(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting join tokens from swarm inspect",
			"Could not get join tokens from swarm inspect, unexpected error: "+err.Error(),
		)
		return
	}
	state := swarmInitResourceModel{
		ID:            tfTypes.StringValue(swarmInfo.ID),
		AdvertiseAddr: plan.AdvertiseAddr,
		ListenAddr:    plan.ListenAddr,
		ManagerToken:  tfTypes.StringValue(swarmInfoWithTokens.JoinTokens.Manager),
		WorkerToken:   tfTypes.StringValue(swarmInfoWithTokens.JoinTokens.Worker),
		Node: &swarmInitNodeModel{
			Host:         plan.Node.Host,
			Context:      plan.Node.Context,
			SSHOpts:      plan.Node.SSHOpts,
			CertMaterial: plan.Node.CertMaterial,
			KeyMaterial:  plan.Node.KeyMaterial,
			CaMaterial:   plan.Node.CaMaterial,
			CertPath:     plan.Node.CertPath,
		},
	}
	diags = resp.State.Set(ctx, &state)
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

	// Recreate Docker client from state.Node
	var sshOpts []string
	if state.Node != nil && state.Node.SSHOpts.Elements() != nil {
		for _, opt := range state.Node.SSHOpts.Elements() {
			sshOpts = append(sshOpts, opt.String())
		}
	}
	dockerConfig := &docker.Config{
		Host:     state.Node.Host.ValueString(),
		SSHOpts:  sshOpts,
		Cert:     state.Node.CertMaterial.ValueString(),
		Key:      state.Node.KeyMaterial.ValueString(),
		Ca:       state.Node.CaMaterial.ValueString(),
		CertPath: state.Node.CertPath.ValueString(),
	}
	dockerClient, err := dockerConfig.NewClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Docker Client in Read",
			"An unexpected error occurred when creating the Docker client in Read. \n\nDocker Client Error: "+err.Error(),
		)
		return
	}

	// Get current swarm info
	swarmInfo, err := dockerClient.SwarmInspect(ctx)
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

	// Get join tokens from swarm info (they are included in SwarmInspect response)
	state.ManagerToken = tfTypes.StringValue(swarmInfo.JoinTokens.Manager)
	state.WorkerToken = tfTypes.StringValue(swarmInfo.JoinTokens.Worker)

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

	// Recreate Docker client from state.Node if needed
	if r.client == nil {
		var sshOpts []string
		if state.Node != nil && state.Node.SSHOpts.Elements() != nil {
			for _, opt := range state.Node.SSHOpts.Elements() {
				sshOpts = append(sshOpts, opt.String())
			}
		}
		dockerConfig := &docker.Config{
			Host:     state.Node.Host.ValueString(),
			SSHOpts:  sshOpts,
			Cert:     state.Node.CertMaterial.ValueString(),
			Key:      state.Node.KeyMaterial.ValueString(),
			Ca:       state.Node.CaMaterial.ValueString(),
			CertPath: state.Node.CertPath.ValueString(),
		}
		dockerClient, err := dockerConfig.NewClient()
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Create Docker Client in Delete",
				"An unexpected error occurred when creating the Docker client in Delete. \n\nDocker Client Error: "+err.Error(),
			)
			return
		}
		r.client = dockerClient
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
