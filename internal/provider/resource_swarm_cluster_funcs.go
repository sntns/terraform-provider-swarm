package provider

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"time"

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

// pollInterval separates two looks at a cluster that is changing: a node
// joining, draining or being demoted.
var pollInterval = 2 * time.Second

// updateAttempts bounds the tries of a node update that Swarm rejects
// because the node changed since it was read.
const updateAttempts = 5

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
// inits one. Then the nodes that are in no swarm join it, managers first;
// promotions and the other changes of spec come before demotions; and the
// nodes the prior state had that the plan no longer has are removed. prior
// is nil on create.
func reconcile(ctx context.Context, plan, prior *clusterModel) diag.Diagnostics {
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
		case state.LocalNodeState != swarm.LocalNodeStateActive || !state.ControlAvailable || state.Cluster == nil:
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
	if prior != nil && prior.ID.ValueString() != "" && clusterID != "" && clusterID != prior.ID.ValueString() {
		diags.AddAttributeError(path.Root("nodes"), "Another swarm",
			fmt.Sprintf("The managers are in the swarm %s, not in %s, which the state holds.", clusterID, prior.ID.ValueString()))
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
		if diags.Append(refresh(ctx, daemons[manager], path.Root("nodes").AtMapKey(manager))...); diags.HasError() {
			return diags
		}
	}

	if diags.Append(join(ctx, plan, names, daemons, daemons[manager])...); diags.HasError() {
		return diags
	}
	for _, demotions := range []bool{false, true} {
		for _, name := range names {
			if diags.Append(setSpec(ctx, daemons[manager].api, daemons[name].info.Swarm.NodeID, plan.Nodes[name], demotions, path.Root("nodes").AtMapKey(name))...); diags.HasError() {
				return diags
			}
		}
		if !demotions {
			// The demotions come once the promoted nodes are managers, and go through one that stays a manager.
			if diags.Append(promoted(ctx, plan, names, daemons)...); diags.HasError() {
				return diags
			}
			manager = firstManager(plan.Nodes, names)
		}
	}
	if prior != nil {
		if diags.Append(remove(ctx, plan, prior, daemons, daemons[manager])...); diags.HasError() {
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

// refresh reads again what a daemon says of itself.
func refresh(ctx context.Context, d *daemon, at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	result, err := d.api.Info(ctx, client.InfoOptions{})
	if err != nil {
		diags.AddAttributeError(at, "Unreachable node", err.Error())
		return diags
	}
	d.info = result.Info
	return diags
}

// join makes the nodes that are in no swarm join the manager's, managers
// first, each with its role's token. A worker already in a swarm must be in
// that one.
func join(ctx context.Context, plan *clusterModel, names []string, daemons map[string]*daemon, manager *daemon) diag.Diagnostics {
	var diags diag.Diagnostics
	inspected, err := manager.api.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		diags.AddError("Swarm inspect failed", err.Error())
		return diags
	}
	listed, err := manager.api.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		diags.AddError("Node list failed", err.Error())
		return diags
	}
	members := map[string]bool{}
	var addresses []string
	for _, node := range listed.Items {
		members[node.ID] = true
		if node.ManagerStatus != nil {
			addresses = append(addresses, node.ManagerStatus.Addr)
		}
	}

	ordered := slices.Clone(names)
	slices.SortStableFunc(ordered, func(a, b string) int { return cmp.Compare(rank(plan.Nodes[b].Role), rank(plan.Nodes[a].Role)) })
	for _, name := range ordered {
		d, at := daemons[name], path.Root("nodes").AtMapKey(name)
		if d.info.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
			if !members[d.info.Swarm.NodeID] {
				diags.AddAttributeError(at, "Node of another swarm",
					"The node is in a swarm whose managers are not in the configuration: make it leave that swarm (`docker swarm leave`) first.")
				return diags
			}
			continue
		}
		token := inspected.Swarm.JoinTokens.Worker
		if plan.Nodes[name].Role.ValueString() == string(swarm.NodeRoleManager) {
			token = inspected.Swarm.JoinTokens.Manager
		}
		if d.info.Swarm.LocalNodeState == swarm.LocalNodeStateInactive {
			_, err := d.api.SwarmJoin(ctx, client.SwarmJoinOptions{
				ListenAddr:    "0.0.0.0:2377",
				AdvertiseAddr: plan.Nodes[name].AdvertiseAddr.ValueString(),
				RemoteAddrs:   addresses,
				JoinToken:     token,
			})
			if err != nil {
				tflog.Warn(ctx, "the join did not complete yet", map[string]any{"node": name, "error": err.Error()})
			}
		}
		// A join that takes long goes on in the background: the node is pending until it completes.
		for {
			if diags.Append(refresh(ctx, d, at)...); diags.HasError() {
				return diags
			}
			state := d.info.Swarm.LocalNodeState
			if state == swarm.LocalNodeStateActive {
				break
			}
			if state != swarm.LocalNodeStatePending {
				diags.AddAttributeError(at, "Swarm join failed", fmt.Sprintf("The node is %s after joining: %s", state, d.info.Swarm.Error))
				return diags
			}
			if !pause(ctx) {
				diags.AddAttributeError(at, "Swarm join failed", "The node was still joining when the timeout ended.")
				return diags
			}
		}
		tflog.Info(ctx, "the node joined the swarm", map[string]any{"node": name})
	}
	return diags
}

// promoted waits until every node the plan makes a manager is one.
func promoted(ctx context.Context, plan *clusterModel, names []string, daemons map[string]*daemon) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, name := range names {
		if plan.Nodes[name].Role.ValueString() != string(swarm.NodeRoleManager) {
			continue
		}
		at := path.Root("nodes").AtMapKey(name)
		for {
			if diags.Append(refresh(ctx, daemons[name], at)...); diags.HasError() || daemons[name].info.Swarm.ControlAvailable {
				break
			}
			if !pause(ctx) {
				diags.AddAttributeError(at, "Promotion not finished", "The node was not a manager yet when the timeout ended.")
				break
			}
		}
		if diags.HasError() {
			return diags
		}
	}
	return diags
}

// setSpec sets the node's spec as the plan gives it, in the pass of the
// demotions or in the pass of every other change.
func setSpec(ctx context.Context, api *client.Client, id string, node nodeModel, demotions bool, at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	inspected, err := api.NodeInspect(ctx, id, client.NodeInspectOptions{})
	if err != nil {
		diags.AddAttributeError(at, "Node not found", err.Error())
		return diags
	}
	spec, changed, d := desiredSpec(ctx, inspected.Node.Spec, node)
	diags.Append(d...)
	demotion := inspected.Node.Spec.Role == swarm.NodeRoleManager && spec.Role == swarm.NodeRoleWorker
	if diags.HasError() || !changed || demotion != demotions {
		return diags
	}
	diags.Append(update(ctx, api, id, func(current *swarm.NodeSpec) { *current, _, _ = desiredSpec(ctx, *current, node) }, at)...)
	return diags
}

// remove takes out of the cluster the nodes the prior state had and the
// plan no longer has, and the ghosts of nodes that joined again under
// another ID: each is drained until its tasks run elsewhere, demoted, made
// to leave when it answers, then removed. Nodes that joined by hand stay.
func remove(ctx context.Context, plan, prior *clusterModel, daemons map[string]*daemon, manager *daemon) diag.Diagnostics {
	var diags diag.Diagnostics
	current := map[string]bool{}
	for _, d := range daemons {
		current[d.info.Swarm.NodeID] = true
	}
	names := slices.Sorted(maps.Keys(prior.Nodes))
	slices.SortStableFunc(names, func(a, b string) int { return cmp.Compare(rank(prior.Nodes[a].Role), rank(prior.Nodes[b].Role)) })
	for _, name := range names {
		node, at := prior.Nodes[name], path.Root("nodes").AtMapKey(name)
		id := node.NodeID.ValueString()
		if id == "" || current[id] {
			continue
		}
		inspected, err := manager.api.NodeInspect(ctx, id, client.NodeInspectOptions{})
		if err != nil {
			continue
		}
		_, kept := plan.Nodes[name]
		if !kept && inspected.Node.Status.State == swarm.NodeStateReady {
			if diags.Append(update(ctx, manager.api, id, func(spec *swarm.NodeSpec) { spec.Availability = swarm.NodeAvailabilityDrain }, at)...); diags.HasError() {
				return diags
			}
			if diags.Append(drained(ctx, manager.api, id, at)...); diags.HasError() {
				return diags
			}
		}
		if inspected.Node.Spec.Role == swarm.NodeRoleManager {
			if diags.Append(update(ctx, manager.api, id, func(spec *swarm.NodeSpec) { spec.Role = swarm.NodeRoleWorker }, at)...); diags.HasError() {
				return diags
			}
			if diags.Append(demoted(ctx, manager.api, id, at)...); diags.HasError() {
				return diags
			}
		}
		if !kept {
			if d, err := reach(ctx, node.Endpoint.ValueString(), node.tls(prior.TLS), false); err == nil {
				if d.info.Swarm.LocalNodeState == swarm.LocalNodeStateActive && d.info.Swarm.NodeID == id {
					if _, err := d.api.SwarmLeave(ctx, client.SwarmLeaveOptions{}); err != nil {
						tflog.Warn(ctx, "the node did not leave the swarm", map[string]any{"node": name, "error": err.Error()})
					}
				}
				_ = d.api.Close()
			}
		}
		if _, err := manager.api.NodeRemove(ctx, id, client.NodeRemoveOptions{Force: true}); err != nil {
			diags.AddAttributeError(at, "Node removal failed", err.Error())
			return diags
		}
		tflog.Info(ctx, "removed the node from the swarm", map[string]any{"node": name, "id": id})
	}
	return diags
}

// update changes the spec of a node, at its current version. Swarm
// rejects the update when it changed the node since it was read, as its
// status right after a join or a promotion: the node is read again and the
// update tried again.
func update(ctx context.Context, api *client.Client, id string, change func(*swarm.NodeSpec), at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	for attempt := 1; ; attempt++ {
		inspected, err := api.NodeInspect(ctx, id, client.NodeInspectOptions{})
		if err != nil {
			diags.AddAttributeError(at, "Node not found", err.Error())
			return diags
		}
		spec := inspected.Node.Spec
		change(&spec)
		_, err = api.NodeUpdate(ctx, id, client.NodeUpdateOptions{Version: inspected.Node.Version, Spec: spec})
		if err == nil {
			return diags
		}
		if !strings.Contains(err.Error(), "update out of sequence") || attempt == updateAttempts || !pause(ctx) {
			diags.AddAttributeError(at, "Node update failed", err.Error())
			return diags
		}
	}
}

// drained waits until no task runs on the node.
func drained(ctx context.Context, api *client.Client, id string, at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	for {
		tasks, err := api.TaskList(ctx, client.TaskListOptions{Filters: make(client.Filters).Add("node", id).Add("desired-state", "running")})
		if err != nil {
			diags.AddAttributeError(at, "Task list failed", err.Error())
			return diags
		}
		if len(tasks.Items) == 0 {
			return diags
		}
		if !pause(ctx) {
			diags.AddAttributeError(at, "Drain not finished", fmt.Sprintf("%d tasks still run on the node when the timeout ended.", len(tasks.Items)))
			return diags
		}
	}
}

// demoted waits until the node is no longer a manager of the raft.
func demoted(ctx context.Context, api *client.Client, id string, at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	for {
		inspected, err := api.NodeInspect(ctx, id, client.NodeInspectOptions{})
		if err != nil {
			diags.AddAttributeError(at, "Node not found", err.Error())
			return diags
		}
		if inspected.Node.ManagerStatus == nil {
			return diags
		}
		if !pause(ctx) {
			diags.AddAttributeError(at, "Demotion not finished", "The node was still a manager when the timeout ended.")
			return diags
		}
	}
}

// pause waits between two looks at the cluster, and tells whether the
// context lets the wait go on.
func pause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(pollInterval):
		return true
	}
}

// quorumKept tells whether the managers of the state that stay managers in
// the plan, and were ready at the last read, are a majority of the state's
// managers: the raft then keeps its quorum while the others are demoted or
// removed.
func quorumKept(state, plan map[string]nodeModel) (kept, total int, ok bool) {
	for name, node := range state {
		if node.Role.ValueString() != string(swarm.NodeRoleManager) {
			continue
		}
		total++
		if planned, in := plan[name]; in && planned.Role.ValueString() == string(swarm.NodeRoleManager) && node.Status.ValueString() == string(swarm.NodeStateReady) {
			kept++
		}
	}
	return kept, total, total == 0 || kept >= total/2+1
}

// rank orders workers before managers.
func rank(role types.String) int {
	if role.ValueString() == string(swarm.NodeRoleManager) {
		return 1
	}
	return 0
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

// leave makes every node leave the swarm: the managers but one are
// demoted, so that each leaves as a worker without breaking the quorum,
// and the last manager leaves with force, which takes the swarm down.
func leave(ctx context.Context, state *clusterModel) diag.Diagnostics {
	var diags diag.Diagnostics
	names := slices.Sorted(maps.Keys(state.Nodes))
	daemons := map[string]*daemon{}
	defer closeAll(daemons)
	for _, name := range names {
		node := state.Nodes[name]
		d, err := reach(ctx, node.Endpoint.ValueString(), node.tls(state.TLS), true)
		if err != nil {
			diags.AddAttributeError(path.Root("nodes").AtMapKey(name), "Unreachable node", err.Error())
			return diags
		}
		daemons[name] = d
	}
	last := firstManager(state.Nodes, names)
	keeper := daemons[last]
	for _, name := range names {
		d, at := daemons[name], path.Root("nodes").AtMapKey(name)
		if name == last || !d.info.Swarm.ControlAvailable {
			continue
		}
		if diags.Append(update(ctx, keeper.api, d.info.Swarm.NodeID, func(spec *swarm.NodeSpec) { spec.Role = swarm.NodeRoleWorker }, at)...); diags.HasError() {
			return diags
		}
		if diags.Append(demoted(ctx, keeper.api, d.info.Swarm.NodeID, at)...); diags.HasError() {
			return diags
		}
	}
	for _, name := range append(slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == last }), last) {
		d := daemons[name]
		if d.info.Swarm.LocalNodeState == swarm.LocalNodeStateInactive {
			continue
		}
		if _, err := d.api.SwarmLeave(ctx, client.SwarmLeaveOptions{Force: name == last}); err != nil {
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
