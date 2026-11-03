package provider

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// statusAbsent is the status of a node that is in no swarm.
const statusAbsent = "absent"

// tls is the node's TLS, else the cluster's.
func (node nodeModel) tls(cluster *tlsModel) *tlsModel {
	if node.TLS != nil {
		return node.TLS
	}
	return cluster
}

func closeAll(daemons map[string]*daemon) {
	for _, d := range daemons {
		_ = d.api.Close()
	}
}

// reconcile brings the cluster to the plan, from what Docker says: it
// adopts the cluster the managers are in, else the first manager by name
// inits one, then sets each node's spec, and records what Docker reports.
// imported is the cluster's ID, when it is known before the nodes are.
func reconcile(ctx context.Context, plan *clusterModel, imported string) diag.Diagnostics {
	var diags diag.Diagnostics
	names := slices.Sorted(maps.Keys(plan.Nodes))
	daemons := map[string]*daemon{}
	defer closeAll(daemons)
	for _, name := range names {
		node := plan.Nodes[name]
		d, err := reach(ctx, node.Endpoint.ValueString(), node.tls(plan.TLS), true)
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name).AtName("endpoint"), "Unreachable node", err.Error())
			return diags
		}
		daemons[name] = d
	}

	manager, clusterID := "", ""
	for _, name := range names {
		state := daemons[name].info.Swarm
		switch {
		case state.LocalNodeState == swarm.LocalNodeStateLocked:
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Locked node",
				"The node's swarm is autolocked: unlock it with `docker swarm unlock` first.")
			return diags
		case state.LocalNodeState != swarm.LocalNodeStateActive:
			continue
		case !state.ControlAvailable:
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Worker of another swarm",
				fmt.Sprintf("The node is a worker of a swarm whose managers are not in the configuration: make it leave that swarm (`docker swarm leave`) first. %s", state.Error))
			return diags
		case state.Cluster == nil:
			continue
		case clusterID != "" && state.Cluster.ID != clusterID:
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Managers of two swarms",
				fmt.Sprintf("The managers are in two swarms, %s and %s: the provider never merges swarms.", clusterID, state.Cluster.ID))
			return diags
		}
		clusterID = state.Cluster.ID
		if manager == "" {
			manager = name
		}
	}
	if imported != "" && clusterID != "" && clusterID != imported {
		diags.AddAttributeError(path.Root("nodes"), "Another swarm",
			fmt.Sprintf("The managers are in the swarm %s, not in %s, which the state holds.", clusterID, imported))
		return diags
	}

	settings, d := initOptions(ctx, plan.Init)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	if manager == "" {
		manager = firstManager(plan.Nodes, names)
		node := plan.Nodes[manager]
		settings.ListenAddr = "0.0.0.0:2377"
		settings.AdvertiseAddr = node.AdvertiseAddr.ValueString()
		if _, err := daemons[manager].api.SwarmInit(ctx, settings); err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(manager), "Swarm init failed", err.Error())
			return diags
		}
		tflog.Info(ctx, "initialised the swarm", map[string]any{"node": manager})
		result, err := daemons[manager].api.Info(ctx, client.InfoOptions{})
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(manager), "Unreachable node", err.Error())
			return diags
		}
		daemons[manager].info = result.Info
	}

	api := daemons[manager].api
	for _, name := range names {
		id := daemons[name].info.Swarm.NodeID
		inspected, err := api.NodeInspect(ctx, id, client.NodeInspectOptions{})
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Node not found", err.Error())
			return diags
		}
		spec, changed, d := desiredSpec(ctx, inspected.Node.Spec, plan.Nodes[name])
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		if !changed {
			continue
		}
		if _, err := api.NodeUpdate(ctx, id, client.NodeUpdateOptions{Version: inspected.Node.Version, Spec: spec}); err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Node update failed", err.Error())
			return diags
		}
	}

	diags.Append(observe(ctx, plan, daemons, daemons[manager])...)
	if diags.HasError() {
		return diags
	}
	if adopted, d := initOptions(ctx, plan.Init); !d.HasError() && !matches(settings, adopted) {
		diags.AddAttributeError(path.Root("init"), "Init settings of the adopted swarm",
			"The swarm the managers are in was created with other init settings, which Docker cannot change. Give the swarm's, or none.")
	}
	return diags
}

// read records the cluster as Docker reports it. It reports gone when
// the nodes that answer are in no swarm, or in another one. A node that does
// not answer is not reachable, and the read goes on.
func read(ctx context.Context, state *clusterModel) (gone bool, diags diag.Diagnostics) {
	names := slices.Sorted(maps.Keys(state.Nodes))
	daemons := map[string]*daemon{}
	defer closeAll(daemons)
	var manager *daemon
	inSwarm := false
	for _, name := range names {
		node := state.Nodes[name]
		d, err := reach(ctx, node.Endpoint.ValueString(), node.tls(state.TLS), false)
		if err != nil {
			tflog.Warn(ctx, "the node does not answer", map[string]any{"node": name, "error": err.Error()})
			continue
		}
		daemons[name] = d
		if d.info.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
			inSwarm = true
			if manager == nil && d.info.Swarm.ControlAvailable {
				manager = d
			}
		}
	}
	if manager == nil {
		if len(daemons) > 0 && !inSwarm {
			return true, diags
		}
		for name, node := range state.Nodes {
			_, reachable := daemons[name]
			node.Reachable = types.BoolValue(reachable)
			state.Nodes[name] = node
		}
		return false, diags
	}
	if state.ID.ValueString() != "" && manager.info.Swarm.Cluster.ID != state.ID.ValueString() {
		return true, diags
	}
	return false, observe(ctx, state, daemons, manager)
}

// clusterView is a cluster as one of its managers reports it.
type clusterView struct {
	ID               types.String
	Init             types.Object
	JoinTokens       types.Object
	ManagerAddresses types.List
	Nodes            []swarm.Node
}

// inspect reads the cluster from one of its managers.
func inspect(ctx context.Context, api *client.Client) (clusterView, diag.Diagnostics) {
	var view clusterView
	var diags diag.Diagnostics
	inspected, err := api.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		diags.AddError("Swarm inspect failed", err.Error())
		return view, diags
	}
	listed, err := api.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		diags.AddError("Node list failed", err.Error())
		return view, diags
	}
	cluster := inspected.Swarm

	view.ID = types.StringValue(cluster.ID)
	pool := make([]attr.Value, 0, len(cluster.DefaultAddrPool))
	for _, prefix := range cluster.DefaultAddrPool {
		pool = append(pool, types.StringValue(prefix.String()))
	}
	view.Init = types.ObjectValueMust(initTypes, map[string]attr.Value{
		"default_addr_pool": types.ListValueMust(types.StringType, pool),
		"subnet_size":       types.Int64Value(int64(cluster.SubnetSize)),
		"data_path_port":    types.Int64Value(int64(cluster.DataPathPort)),
	})
	view.JoinTokens = types.ObjectValueMust(joinTokensTypes, map[string]attr.Value{
		"manager": types.StringValue(cluster.JoinTokens.Manager),
		"worker":  types.StringValue(cluster.JoinTokens.Worker),
	})
	var addresses []string
	for _, node := range listed.Items {
		if node.ManagerStatus != nil {
			addresses = append(addresses, node.ManagerStatus.Addr)
		}
	}
	slices.Sort(addresses)
	view.ManagerAddresses, diags = types.ListValueFrom(ctx, types.StringType, addresses)
	view.Nodes = listed.Items
	return view, diags
}

// observe records what the manager reports of the cluster and its nodes.
// A node is Docker's whose ID its daemon gives, else the one the state
// holds, for a node that does not answer.
func observe(ctx context.Context, model *clusterModel, daemons map[string]*daemon, manager *daemon) diag.Diagnostics {
	view, diags := inspect(ctx, manager.api)
	if diags.HasError() {
		return diags
	}
	model.ID, model.Init, model.JoinTokens, model.ManagerAddresses = view.ID, view.Init, view.JoinTokens, view.ManagerAddresses
	byID := map[string]swarm.Node{}
	for _, node := range view.Nodes {
		byID[node.ID] = node
	}

	for name, node := range model.Nodes {
		id := node.NodeID.ValueString()
		answered, reachable := daemons[name]
		if reachable {
			id = ""
			if answered.info.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
				id = answered.info.Swarm.NodeID
			}
		}
		node.Reachable = types.BoolValue(reachable)
		found, ok := byID[id]
		if !ok {
			node.NodeID = types.StringNull()
			node.Status = types.StringValue(statusAbsent)
			model.Nodes[name] = node
			continue
		}
		labels, d := types.MapValueFrom(ctx, types.StringType, orEmpty(found.Spec.Labels))
		diags.Append(d...)
		node.NodeID = types.StringValue(found.ID)
		node.Role = types.StringValue(string(found.Spec.Role))
		node.Availability = types.StringValue(string(found.Spec.Availability))
		node.Labels = labels
		node.Status = types.StringValue(string(found.Status.State))
		model.Nodes[name] = node
	}
	return diags
}

// desiredSpec is the node's spec as the configuration gives it: its role,
// its availability, and its labels, which replace Docker's.
func desiredSpec(ctx context.Context, current swarm.NodeSpec, node nodeModel) (swarm.NodeSpec, bool, diag.Diagnostics) {
	labels := map[string]string{}
	diags := node.Labels.ElementsAs(ctx, &labels, false)
	desired := current
	desired.Role = swarm.NodeRole(node.Role.ValueString())
	desired.Availability = swarm.NodeAvailability(node.Availability.ValueString())
	desired.Labels = labels
	changed := desired.Role != current.Role || desired.Availability != current.Availability || !maps.Equal(labels, orEmpty(current.Labels))
	return desired, changed, diags
}

// leave makes every node leave the swarm, workers first, the managers
// with force, since the last one takes the swarm down with it.
func leave(ctx context.Context, state *clusterModel) diag.Diagnostics {
	var diags diag.Diagnostics
	rank := func(name string) int {
		if state.Nodes[name].Role.ValueString() == string(swarm.NodeRoleManager) {
			return 1
		}
		return 0
	}
	names := slices.Sorted(maps.Keys(state.Nodes))
	slices.SortStableFunc(names, func(a, b string) int { return cmp.Compare(rank(a), rank(b)) })
	for _, name := range names {
		node := state.Nodes[name]
		d, err := reach(ctx, node.Endpoint.ValueString(), node.tls(state.TLS), true)
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Unreachable node", err.Error())
			return diags
		}
		manager := node.Role.ValueString() == string(swarm.NodeRoleManager)
		if d.info.Swarm.LocalNodeState != swarm.LocalNodeStateInactive {
			_, err = d.api.SwarmLeave(ctx, client.SwarmLeaveOptions{Force: manager})
		}
		_ = d.api.Close()
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Swarm leave failed", err.Error())
			return diags
		}
	}
	return diags
}

// firstManager is the first node by name whose role is manager.
func firstManager(nodes map[string]nodeModel, names []string) string {
	for _, name := range names {
		if nodes[name].Role.ValueString() == string(swarm.NodeRoleManager) {
			return name
		}
	}
	return names[0]
}

// initOptions are the init settings the configuration gives; Docker
// chooses the others.
func initOptions(ctx context.Context, given types.Object) (client.SwarmInitOptions, diag.Diagnostics) {
	var options client.SwarmInitOptions
	if given.IsNull() || given.IsUnknown() {
		return options, nil
	}
	var settings struct {
		DefaultAddrPool types.List  `tfsdk:"default_addr_pool"`
		SubnetSize      types.Int64 `tfsdk:"subnet_size"`
		DataPathPort    types.Int64 `tfsdk:"data_path_port"`
	}
	diags := given.As(ctx, &settings, basetypes.ObjectAsOptions{UnhandledUnknownAsEmpty: true})
	var pool []string
	if !settings.DefaultAddrPool.IsUnknown() {
		diags.Append(settings.DefaultAddrPool.ElementsAs(ctx, &pool, false)...)
	}
	for _, cidr := range pool {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			diags.AddAttributeError(path.Root("init").AtName("default_addr_pool"), "Invalid CIDR", err.Error())
			continue
		}
		options.DefaultAddrPool = append(options.DefaultAddrPool, prefix)
	}
	options.SubnetSize = uint32(max(settings.SubnetSize.ValueInt64(), 0))
	options.DataPathPort = uint32(max(settings.DataPathPort.ValueInt64(), 0))
	return options, diags
}

// matches tells whether the swarm's settings are those the configuration
// gives: the ones it leaves to Docker match any.
func matches(given, swarm client.SwarmInitOptions) bool {
	return (len(given.DefaultAddrPool) == 0 || slices.Equal(given.DefaultAddrPool, swarm.DefaultAddrPool)) &&
		(given.SubnetSize == 0 || given.SubnetSize == swarm.SubnetSize) &&
		(given.DataPathPort == 0 || given.DataPathPort == swarm.DataPathPort)
}

func orEmpty(labels map[string]string) map[string]string {
	if labels == nil {
		return map[string]string{}
	}
	return labels
}
