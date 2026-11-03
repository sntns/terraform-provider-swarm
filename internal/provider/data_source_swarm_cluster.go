package provider

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/moby/moby/api/types/swarm"
)

var _ datasource.DataSource = &swarmClusterDataSource{}

type swarmClusterDataSource struct{}

type clusterDataSourceModel struct {
	Endpoint         types.String           `tfsdk:"endpoint"`
	TLS              *tlsModel              `tfsdk:"tls"`
	ID               types.String           `tfsdk:"id"`
	Init             types.Object           `tfsdk:"init"`
	ManagerAddresses types.List             `tfsdk:"manager_addresses"`
	JoinTokens       types.Object           `tfsdk:"join_tokens"`
	Nodes            []clusterDataNodeModel `tfsdk:"nodes"`
}

type clusterDataNodeModel struct {
	NodeID       types.String `tfsdk:"node_id"`
	Hostname     types.String `tfsdk:"hostname"`
	Address      types.String `tfsdk:"address"`
	Role         types.String `tfsdk:"role"`
	Availability types.String `tfsdk:"availability"`
	Labels       types.Map    `tfsdk:"labels"`
	Status       types.String `tfsdk:"status"`
}

// NewSwarmClusterDataSource returns the swarm_cluster data source.
func NewSwarmClusterDataSource() datasource.DataSource {
	return &swarmClusterDataSource{}
}

func (d *swarmClusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *swarmClusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Docker Swarm cluster from one of its managers: its tokens and managers' addresses, " +
			"for a configuration that joins nodes the `swarm_cluster` resource does not manage, and its nodes as Docker sees them.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "A manager's Docker: `tcp://HOST:PORT`, in TLS when `tls` is given, `ssh://USER@HOST` or `unix:///PATH`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^(tcp|ssh|unix)://.`), "must be a tcp://, ssh:// or unix:// endpoint")},
			},
			"tls": schema.SingleNestedAttribute{
				MarkdownDescription: "The TLS of a `tcp://` endpoint.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"ca_pem":   schema.StringAttribute{MarkdownDescription: "The CA of the daemon's certificate, in PEM. Default: the system's.", Optional: true},
					"cert_pem": schema.StringAttribute{MarkdownDescription: "The client's certificate, in PEM.", Optional: true},
					"key_pem":  schema.StringAttribute{MarkdownDescription: "The client's key, in PEM.", Optional: true, Sensitive: true},
				},
			},
			"id": computed("The cluster's ID."),
			"init": schema.SingleNestedAttribute{
				MarkdownDescription: "The settings the cluster was inited with.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"default_addr_pool": schema.ListAttribute{MarkdownDescription: "The pools of the overlay networks' subnets.", ElementType: types.StringType, Computed: true},
					"subnet_size":       schema.Int64Attribute{MarkdownDescription: "The prefix length of each overlay network's subnet.", Computed: true},
					"data_path_port":    schema.Int64Attribute{MarkdownDescription: "The UDP port of the overlay networks' traffic.", Computed: true},
				},
			},
			"manager_addresses": schema.ListAttribute{
				MarkdownDescription: "The managers' addresses, `HOST:PORT`, that nodes join.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"join_tokens": schema.SingleNestedAttribute{
				MarkdownDescription: "The tokens to join the cluster with.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"manager": schema.StringAttribute{MarkdownDescription: "A manager's.", Computed: true, Sensitive: true},
					"worker":  schema.StringAttribute{MarkdownDescription: "A worker's.", Computed: true, Sensitive: true},
				},
			},
			"nodes": schema.ListNestedAttribute{
				MarkdownDescription: "The cluster's nodes, by hostname.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node_id":      computed("The node's ID."),
						"hostname":     computed("The node's hostname, as its machine gives it."),
						"address":      computed("The address the managers see the node at."),
						"role":         computed("`manager` or `worker`."),
						"availability": computed("`active`, `pause` or `drain`."),
						"labels":       schema.MapAttribute{MarkdownDescription: "The node's labels.", ElementType: types.StringType, Computed: true},
						"status":       computed("`ready`, `down`, `disconnected` or `unknown`."),
					},
				},
			},
		},
	}
}

func (d *swarmClusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	manager, err := reach(ctx, config.Endpoint.ValueString(), config.TLS, false)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unreachable Docker", err.Error())
		return
	}
	defer func() { _ = manager.api.Close() }()
	if state := manager.info.Swarm; state.LocalNodeState != swarm.LocalNodeStateActive || !state.ControlAvailable {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Not a manager",
			fmt.Sprintf("The Docker at %s is not a manager of a swarm: it is %s.", config.Endpoint.ValueString(), cmp.Or(string(state.LocalNodeState), "inactive")))
		return
	}

	view, diags := inspect(ctx, manager.api)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.ID, config.Init, config.JoinTokens, config.ManagerAddresses = view.ID, view.Init, view.JoinTokens, view.ManagerAddresses
	nodes := slices.SortedFunc(slices.Values(view.Nodes), func(a, b swarm.Node) int {
		return cmp.Or(cmp.Compare(a.Description.Hostname, b.Description.Hostname), cmp.Compare(a.ID, b.ID))
	})
	config.Nodes = make([]clusterDataNodeModel, 0, len(nodes))
	for _, node := range nodes {
		labels, diags := types.MapValueFrom(ctx, types.StringType, orEmpty(node.Spec.Labels))
		resp.Diagnostics.Append(diags...)
		config.Nodes = append(config.Nodes, clusterDataNodeModel{
			NodeID:       types.StringValue(node.ID),
			Hostname:     types.StringValue(node.Description.Hostname),
			Address:      types.StringValue(node.Status.Addr),
			Role:         types.StringValue(string(node.Spec.Role)),
			Availability: types.StringValue(string(node.Spec.Availability)),
			Labels:       labels,
			Status:       types.StringValue(string(node.Status.State)),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
