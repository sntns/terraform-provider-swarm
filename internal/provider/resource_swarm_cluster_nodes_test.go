package provider

import (
	"context"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuorumKept(t *testing.T) {
	t.Parallel()
	node := func(role, status string) nodeModel {
		return nodeModel{Role: types.StringValue(role), Status: types.StringValue(status)}
	}
	three := map[string]nodeModel{"a": node("manager", "ready"), "b": node("manager", "ready"), "c": node("manager", "ready"), "w": node("worker", "ready")}

	_, _, ok := quorumKept(three, map[string]nodeModel{"a": node("manager", ""), "b": node("manager", ""), "c": node("worker", "")})
	assert.True(t, ok, "2 of 3 managers stay")
	kept, total, ok := quorumKept(three, map[string]nodeModel{"a": node("manager", ""), "d": node("manager", "")})
	assert.False(t, ok, "1 of 3 managers stays: a new manager does not count until it joined")
	assert.Equal(t, []int{1, 3}, []int{kept, total})
	down := map[string]nodeModel{"a": node("manager", "ready"), "b": node("manager", "down"), "c": node("manager", "ready")}
	_, _, ok = quorumKept(down, map[string]nodeModel{"a": node("manager", ""), "b": node("manager", "")})
	assert.False(t, ok, "a manager that is down does not keep the quorum")
	_, _, ok = quorumKept(map[string]nodeModel{"a": node("manager", "ready")}, map[string]nodeModel{"a": node("manager", ""), "b": node("manager", "")})
	assert.True(t, ok, "adding managers keeps the quorum")
}

// Four daemons in docker:dind: three managers and a worker join, two swap
// roles, one leaves the configuration, a manager is lost, a change that
// would lose the quorum is refused, the lost manager is removed, and the
// others leave when the cluster is destroyed.
func TestAccSwarmCluster_nodes(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("the acceptance tests run with %s set", resource.EnvTfAcc)
	}
	nodes := startNodes(t, 4)
	api := testDocker(t)
	config := func(roles map[string]string) string {
		var lines []string
		for _, name := range slices.Sorted(maps.Keys(roles)) {
			node := nodes[name[len(name)-1]-'1']
			lines = append(lines, fmt.Sprintf("    %q = { endpoint = %q, advertise_addr = %q, role = %q }", name, node.endpoint, node.address, roles[name]))
		}
		return fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_nodes"), strings.Join(lines, "\n"))
	}
	daemonOf := func(i int) *client.Client {
		d, err := connect(nodes[i].endpoint, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = d.Close() })
		return d
	}
	first := daemonOf(0)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			for _, i := range []int{0, 3} {
				info, err := daemonOf(i).Info(context.Background(), client.InfoOptions{})
				if err != nil {
					return err
				}
				if info.Info.Swarm.LocalNodeState != swarm.LocalNodeStateInactive {
					return fmt.Errorf("node-%d is still in the swarm", i+1)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config(map[string]string{"node-1": "manager", "node-2": "manager", "node-3": "manager", "node-4": "worker"}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("swarm_cluster.test", "manager_addresses.#", "3"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-4.status", "ready"),
					checkRoles(first, map[string]swarm.NodeRole{nodes[0].address: "manager", nodes[1].address: "manager", nodes[2].address: "manager", nodes[3].address: "worker"}),
				),
			},
			{
				Config: config(map[string]string{"node-1": "manager", "node-2": "manager", "node-3": "worker", "node-4": "manager"}),
				Check:  checkRoles(first, map[string]swarm.NodeRole{nodes[0].address: "manager", nodes[1].address: "manager", nodes[2].address: "worker", nodes[3].address: "manager"}),
			},
			{
				Config: config(map[string]string{"node-1": "manager", "node-2": "manager", "node-4": "manager"}),
				Check: resource.ComposeTestCheckFunc(
					checkRoles(first, map[string]swarm.NodeRole{nodes[0].address: "manager", nodes[1].address: "manager", nodes[3].address: "manager"}),
					checkLeft(daemonOf(2), "node-3"),
				),
			},
			{
				PreConfig: func() {
					_, err := api.ContainerStop(context.Background(), nodes[1].container, client.ContainerStopOptions{})
					require.NoError(t, err)
				},
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-2.reachable", "false"),
			},
			{
				Config:      config(map[string]string{"node-4": "manager"}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Quorum lost`),
			},
			{
				Config: config(map[string]string{"node-1": "manager", "node-4": "manager"}),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("swarm_cluster.test", "manager_addresses.#", "2"),
					checkRoles(first, map[string]swarm.NodeRole{nodes[0].address: "manager", nodes[3].address: "manager"}),
				),
			},
		},
	})
}

// checkRoles checks the cluster's nodes, by address, and their roles, as
// a manager lists them.
func checkRoles(api *client.Client, roles map[string]swarm.NodeRole) resource.TestCheckFunc {
	return func(*terraform.State) error {
		listed, err := api.NodeList(context.Background(), client.NodeListOptions{})
		if err != nil {
			return err
		}
		found := map[string]swarm.NodeRole{}
		for _, node := range listed.Items {
			found[node.Status.Addr] = node.Spec.Role
		}
		if fmt.Sprint(found) != fmt.Sprint(roles) {
			return fmt.Errorf("the cluster's nodes are %v, not %v", found, roles)
		}
		return nil
	}
}

// checkLeft checks that a node's daemon is in no swarm.
func checkLeft(api *client.Client, name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		info, err := api.Info(context.Background(), client.InfoOptions{})
		if err != nil {
			return err
		}
		if info.Info.Swarm.LocalNodeState != swarm.LocalNodeStateInactive {
			return fmt.Errorf("%s is still in a swarm: %s", name, info.Info.Swarm.LocalNodeState)
		}
		return nil
	}
}
