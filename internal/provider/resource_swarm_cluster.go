package provider

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                   = &swarmClusterResource{}
	_ resource.ResourceWithImportState    = &swarmClusterResource{}
	_ resource.ResourceWithModifyPlan     = &swarmClusterResource{}
	_ resource.ResourceWithValidateConfig = &swarmClusterResource{}
	_ resource.ResourceWithIdentity       = &swarmClusterResource{}
)

const (
	defaultTimeout     = 20 * time.Minute
	defaultReadTimeout = 5 * time.Minute
)

type swarmClusterResource struct{}

type clusterModel struct {
	ID               types.String         `tfsdk:"id"`
	Nodes            map[string]nodeModel `tfsdk:"nodes"`
	TLS              *tlsModel            `tfsdk:"tls"`
	Init             types.Object         `tfsdk:"init"`
	LeaveOnDestroy   types.Bool           `tfsdk:"leave_on_destroy"`
	ManagerAddresses types.List           `tfsdk:"manager_addresses"`
	JoinTokens       types.Object         `tfsdk:"join_tokens"`
	Timeouts         timeouts.Value       `tfsdk:"timeouts"`
}

// clusterIdentityModel identifies a cluster by its ID, for an import block
// that gives an identity rather than an ID.
type clusterIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

type nodeModel struct {
	Endpoint      types.String `tfsdk:"endpoint"`
	AdvertiseAddr types.String `tfsdk:"advertise_addr"`
	Role          types.String `tfsdk:"role"`
	Labels        types.Map    `tfsdk:"labels"`
	Availability  types.String `tfsdk:"availability"`
	TLS           *tlsModel    `tfsdk:"tls"`
	NodeID        types.String `tfsdk:"node_id"`
	Reachable     types.Bool   `tfsdk:"reachable"`
	Status        types.String `tfsdk:"status"`
}

type tlsModel struct {
	CAPEM   types.String `tfsdk:"ca_pem"`
	CertPEM types.String `tfsdk:"cert_pem"`
	KeyPEM  types.String `tfsdk:"key_pem"`
}

var (
	initTypes = map[string]attr.Type{
		"default_addr_pool": types.ListType{ElemType: types.StringType},
		"subnet_size":       types.Int64Type,
		"data_path_port":    types.Int64Type,
	}
	joinTokensTypes = map[string]attr.Type{"manager": types.StringType, "worker": types.StringType}
)

// NewSwarmClusterResource returns the swarm_cluster resource.
func NewSwarmClusterResource() resource.Resource {
	return &swarmClusterResource{}
}

func (r *swarmClusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *swarmClusterResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{Description: "The cluster's ID, as Docker gives it.", RequiredForImport: true},
		},
	}
}

func (r *swarmClusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	tlsAttribute := func(description string) schema.SingleNestedAttribute {
		return schema.SingleNestedAttribute{
			MarkdownDescription: description,
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"ca_pem":   schema.StringAttribute{MarkdownDescription: "The CA of the daemon's certificate, in PEM. Default: the system's.", Optional: true},
				"cert_pem": schema.StringAttribute{MarkdownDescription: "The client's certificate, in PEM.", Optional: true},
				"key_pem":  schema.StringAttribute{MarkdownDescription: "The client's key, in PEM.", Optional: true, Sensitive: true},
			},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Docker Swarm cluster and its nodes. Each create and update reconciles the cluster from what Docker says: " +
			"it adopts the cluster its managers are in, or the first manager by name inits one; the nodes in no swarm join it, managers first; " +
			"promotions come before demotions; and the nodes the configuration no longer has are drained until their tasks run elsewhere, demoted, made to leave, and removed. " +
			"A change that would keep fewer than a majority of the managers ready is refused at plan time: add the new managers in one apply, remove the old ones in the next. " +
			"Destroying it keeps the cluster, unless `leave_on_destroy` is set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The cluster's ID, as Docker gives it.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"nodes": schema.MapNestedAttribute{
				MarkdownDescription: "The nodes, by a name of the configuration's choosing, which is not Docker's hostname. At least one is a `manager`. " +
					"A node is matched to Docker's by the ID its daemon gives, so a changed endpoint that answers with the same ID is the same node.",
				Required:   true,
				Validators: []validator.Map{mapvalidator.SizeAtLeast(1)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"endpoint": schema.StringAttribute{
							MarkdownDescription: "The node's Docker: `tcp://HOST:PORT`, in TLS when `tls` is given, `ssh://USER@HOST` or `unix:///PATH`. " +
								"It may be unknown at plan time, when the machine is created in the same apply: the provider waits until it answers.",
							Required:   true,
							Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^(tcp|ssh|unix)://.`), "must be a tcp://, ssh:// or unix:// endpoint")},
						},
						"advertise_addr": schema.StringAttribute{
							MarkdownDescription: "The address the other nodes reach this one on. Default: Docker's choice.",
							Optional:            true,
						},
						"role": schema.StringAttribute{
							MarkdownDescription: "`manager` or `worker`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("worker"),
							Validators:          []validator.String{stringvalidator.OneOf("manager", "worker")},
						},
						"labels": schema.MapAttribute{
							MarkdownDescription: "The node's labels. Authoritative: a label the configuration does not give is removed.",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							Default:             mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
						},
						"availability": schema.StringAttribute{
							MarkdownDescription: "`active`, `pause` or `drain`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("active"),
							Validators:          []validator.String{stringvalidator.OneOf("active", "pause", "drain")},
						},
						"tls": tlsAttribute("This node's TLS, instead of the cluster's `tls`."),
						"node_id": schema.StringAttribute{
							MarkdownDescription: "The node's ID in the cluster.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"reachable": schema.BoolAttribute{
							MarkdownDescription: "Whether the node's Docker answered when the cluster was last read.",
							Computed:            true,
						},
						"status": schema.StringAttribute{
							MarkdownDescription: "The node's state as the managers see it: `ready`, `down`, `disconnected` or `unknown`; `absent` when it is in no swarm.",
							Computed:            true,
						},
					},
				},
			},
			"tls": tlsAttribute("The TLS of the nodes' `tcp://` endpoints, for those that give none."),
			"init": schema.SingleNestedAttribute{
				MarkdownDescription: "The settings of `docker swarm init`, used when the cluster is created. Docker cannot change them afterwards: a change is refused.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"default_addr_pool": schema.ListAttribute{
						MarkdownDescription: "The pools of the overlay networks' subnets, in CIDR. Default: Docker's, `10.0.0.0/8`.",
						ElementType:         types.StringType,
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
					},
					"subnet_size": schema.Int64Attribute{
						MarkdownDescription: "The prefix length of each overlay network's subnet. Default: Docker's, 24.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
					},
					"data_path_port": schema.Int64Attribute{
						MarkdownDescription: "The UDP port of the overlay networks' traffic. Default: Docker's, 4789.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
					},
				},
			},
			"leave_on_destroy": schema.BoolAttribute{
				MarkdownDescription: "Whether destroying the resource makes every node leave the swarm. Default: `false`, the cluster stays and Terraform forgets it.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"manager_addresses": schema.ListAttribute{
				MarkdownDescription: "The managers' addresses, `HOST:PORT`, that nodes join.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"join_tokens": schema.SingleNestedAttribute{
				MarkdownDescription: "The tokens to join the cluster with.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"manager": schema.StringAttribute{MarkdownDescription: "A manager's.", Computed: true, Sensitive: true},
					"worker":  schema.StringAttribute{MarkdownDescription: "A worker's.", Computed: true, Sensitive: true},
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Read: true, Update: true, Delete: true}),
		},
	}
}

// ValidateConfig refuses a cluster without a manager.
func (r *swarmClusterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var nodes types.Map
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("nodes"), &nodes)...)
	if resp.Diagnostics.HasError() || nodes.IsNull() || nodes.IsUnknown() {
		return
	}
	for _, element := range nodes.Elements() {
		node, ok := element.(basetypes.ObjectValue)
		if !ok || node.IsUnknown() {
			return
		}
		role, ok := node.Attributes()["role"].(basetypes.StringValue)
		if !ok || role.IsUnknown() || role.ValueString() == "manager" {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(path.Root("nodes"), "No manager", "At least one node must have the role `manager`.")
}

// ModifyPlan refuses a change of the init settings and a change that would
// lose the raft's quorum, and plans an update for a node that the last read
// found in no swarm, so that it joins again.
func (r *swarmClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var planned, current types.Object
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("init"), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("init"), &current)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !planned.IsUnknown() && !current.IsNull() && !planned.Equal(current) {
		resp.Diagnostics.AddAttributeError(path.Root("init"), "Init settings changed",
			"Docker cannot change the settings of `docker swarm init` once the cluster runs. Keep the cluster's, or create a new cluster.")
		return
	}

	var nodes types.Map
	var state clusterModel
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("nodes"), &nodes)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || nodes.IsUnknown() {
		return
	}
	plannedNodes := map[string]nodeModel{}
	resp.Diagnostics.Append(nodes.ElementsAs(ctx, &plannedNodes, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if kept, total, ok := quorumKept(state.Nodes, plannedNodes); !ok {
		resp.Diagnostics.AddAttributeError(path.Root("nodes"), "Quorum lost",
			fmt.Sprintf("The plan keeps %d of the cluster's %d managers ready: the raft needs %d of them while the others are demoted or removed. "+
				"Add the new managers in one apply, then demote or remove the old ones in the next.", kept, total, total/2+1))
		return
	}
	for name, node := range state.Nodes {
		if _, planned := nodes.Elements()[name]; planned && node.NodeID.IsNull() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("nodes").AtMapKey(name).AtName("node_id"), types.StringUnknown())...)
		}
	}
}

func (r *swarmClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp.Diagnostics.Append(reconcile(ctx, &plan, nil)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIdentity(ctx, resp.Identity, plan.ID, &resp.Diagnostics)
}

func (r *swarmClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(state.Nodes) == 0 {
		setIdentity(ctx, resp.Identity, state.ID, &resp.Diagnostics)
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultReadTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	gone, diags := read(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if gone {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIdentity(ctx, resp.Identity, state.ID, &resp.Diagnostics)
}

func (r *swarmClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Update(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp.Diagnostics.Append(reconcile(ctx, &plan, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIdentity(ctx, resp.Identity, plan.ID, &resp.Diagnostics)
}

func (r *swarmClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !state.LeaveOnDestroy.ValueBool() {
		return
	}
	timeout, diags := state.Timeouts.Delete(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp.Diagnostics.Append(leave(ctx, &state)...)
}

// ImportState takes the cluster's ID, as `docker info` gives it, or its
// identity: the next apply reaches the nodes of the configuration, which
// must be in that cluster, and records them.
func (r *swarmClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)
}

func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.String, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}
	diags.Append(identity.Set(ctx, clusterIdentityModel{ID: id})...)
}
