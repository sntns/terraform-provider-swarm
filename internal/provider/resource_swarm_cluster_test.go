package provider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwarmClusterResource_schema(t *testing.T) {
	t.Parallel()
	var resp fwresource.SchemaResponse
	NewSwarmClusterResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	tokens := resp.Schema.Attributes["join_tokens"].(schema.SingleNestedAttribute).Attributes
	assert.True(t, tokens["manager"].IsSensitive() && tokens["worker"].IsSensitive(), "a token joins the cluster")
	key := resp.Schema.Attributes["tls"].(schema.SingleNestedAttribute).Attributes["key_pem"]
	assert.True(t, key.IsSensitive())
}

func TestDesiredSpec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	node := nodeModel{
		Role:         types.StringValue("manager"),
		Availability: types.StringValue("active"),
		Labels:       types.MapValueMust(types.StringType, map[string]attr.Value{"zone": types.StringValue("a")}),
	}
	current := swarm.NodeSpec{
		Annotations:  swarm.Annotations{Name: "kept", Labels: map[string]string{"zone": "a", "set-by-hand": "1"}},
		Role:         swarm.NodeRoleManager,
		Availability: swarm.NodeAvailabilityActive,
	}

	desired, changed, diags := desiredSpec(ctx, current, node)
	require.False(t, diags.HasError(), diags)
	assert.True(t, changed, "a label the configuration does not give is removed")
	assert.Equal(t, map[string]string{"zone": "a"}, desired.Labels)
	assert.Equal(t, "kept", desired.Name)

	current.Labels = map[string]string{"zone": "a"}
	_, changed, _ = desiredSpec(ctx, current, node)
	assert.False(t, changed)

	node.Labels = types.MapValueMust(types.StringType, map[string]attr.Value{})
	current.Labels = nil
	_, changed, _ = desiredSpec(ctx, current, node)
	assert.False(t, changed, "no labels in Docker are none in the configuration")
}

func TestMatches(t *testing.T) {
	t.Parallel()
	pool := []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")}
	running := client.SwarmInitOptions{DefaultAddrPool: pool, SubnetSize: 24, DataPathPort: 4789}
	assert.True(t, matches(client.SwarmInitOptions{}, running), "settings left to Docker match any")
	assert.True(t, matches(client.SwarmInitOptions{DataPathPort: 4789, DefaultAddrPool: pool}, running))
	assert.False(t, matches(client.SwarmInitOptions{DataPathPort: 4790}, running))
	assert.False(t, matches(client.SwarmInitOptions{DefaultAddrPool: []netip.Prefix{netip.MustParsePrefix("10.30.0.0/16")}}, running))
}

func TestConnect(t *testing.T) {
	t.Parallel()
	_, err := connect("ssh://deploy@10.0.0.13", nil)
	require.NoError(t, err, "the ssh command is run at the first request only")
	_, err = connect("tcp://10.0.0.11:2376", &tlsModel{CAPEM: types.StringValue("not a PEM")})
	assert.ErrorContains(t, err, "ca_pem")
	_, err = connect("tcp://10.0.0.11:2376", &tlsModel{CertPEM: types.StringValue("not a PEM")})
	assert.ErrorContains(t, err, "PEM pair")
}

// A cluster on the test's Docker: inited with its labels and a data path
// port, its labels and availability changed, a label set by hand removed, a
// change of its init settings refused, imported by its ID, and created
// again once its node left the swarm by hand.
func TestAccSwarmCluster_basic(t *testing.T) {
	api := testDocker(t)
	config := func(labels, availability string, port int) string {
		return fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_basic"), testEndpoint(), labels, availability, port)
	}
	var first string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkSwarmLeft(api),
		Steps: []resource.TestStep{
			{
				Config: config(`{ zone = "a" }`, "active", 4790),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("swarm_cluster.test", "id"),
					resource.TestCheckResourceAttrSet("swarm_cluster.test", "nodes.node-1.node_id"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-1.reachable", "true"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-1.status", "ready"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "init.data_path_port", "4790"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "init.subnet_size", "24"),
					resource.TestCheckResourceAttr("swarm_cluster.test", "manager_addresses.#", "1"),
					resource.TestCheckResourceAttrSet("swarm_cluster.test", "join_tokens.worker"),
					checkNodeSpec(api, map[string]string{"zone": "a"}, swarm.NodeAvailabilityActive),
					func(s *terraform.State) error {
						first = s.RootModule().Resources["swarm_cluster.test"].Primary.ID
						return nil
					},
				),
			},
			{
				Config: config(`{ zone = "b" }`, "drain", 4790),
				Check:  checkNodeSpec(api, map[string]string{"zone": "b"}, swarm.NodeAvailabilityDrain),
			},
			{
				PreConfig: func() { setLabelByHand(t, api, "set-by-hand", "1") },
				Config:    config(`{ zone = "b" }`, "drain", 4790),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("swarm_cluster.test", plancheck.ResourceActionUpdate)},
				},
				Check: checkNodeSpec(api, map[string]string{"zone": "b"}, swarm.NodeAvailabilityDrain),
			},
			{
				Config:      config(`{ zone = "b" }`, "drain", 4791),
				ExpectError: regexp.MustCompile(`Init settings changed`),
			},
			{
				ResourceName: "swarm_cluster.test",
				ImportState:  true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || states[0].ID != first {
						return fmt.Errorf("imported %v, not the cluster %s", states, first)
					}
					return nil
				},
			},
			{
				ResourceName:    "swarm_cluster.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
				Config:          config(`{ zone = "b" }`, "drain", 4790),
				// The import knows the cluster's ID only: the apply records the configuration's nodes.
				ExpectNonEmptyPlan: true,
				ImportPlanChecks: resource.ImportPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("swarm_cluster.test", plancheck.ResourceActionUpdate)},
				},
			},
			{
				PreConfig: func() {
					_, err := api.SwarmLeave(context.Background(), client.SwarmLeaveOptions{Force: true})
					require.NoError(t, err)
				},
				Config: config(`{ zone = "b" }`, "drain", 4790),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("swarm_cluster.test", plancheck.ResourceActionCreate)},
				},
				Check: func(s *terraform.State) error {
					if id := s.RootModule().Resources["swarm_cluster.test"].Primary.ID; id == first {
						return errors.New("the swarm the node left is still the cluster")
					}
					return nil
				},
			},
		},
	})
}

// A swarm inited by hand is adopted, with its ID, and stays when the
// resource is destroyed.
func TestAccSwarmCluster_adopt(t *testing.T) {
	api := testDocker(t)
	var inited string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			result, err := api.Info(context.Background(), client.InfoOptions{})
			if err != nil {
				return err
			}
			if result.Info.Swarm.Cluster == nil || result.Info.Swarm.Cluster.ID != inited {
				return errors.New("destroying the resource took the swarm down, without leave_on_destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					result, err := api.SwarmInit(context.Background(), client.SwarmInitOptions{ListenAddr: "0.0.0.0:2377", AdvertiseAddr: "127.0.0.1"})
					require.NoError(t, err)
					info, err := api.Info(context.Background(), client.InfoOptions{})
					require.NoError(t, err)
					require.Equal(t, result.NodeID, info.Info.Swarm.NodeID)
					inited = info.Info.Swarm.Cluster.ID
				},
				Config: fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_adopt"), testEndpoint()),
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						if id := s.RootModule().Resources["swarm_cluster.test"].Primary.ID; id != inited {
							return fmt.Errorf("the cluster is %s, not the swarm inited by hand, %s", id, inited)
						}
						return nil
					},
					resource.TestCheckResourceAttr("swarm_cluster.test", "leave_on_destroy", "false"),
				),
			},
		},
	})
}

// A node that does not answer is recorded as unreachable, and the refresh
// goes on; it is reachable again once it answers.
func TestAccSwarmCluster_unreachable(t *testing.T) {
	api := testDocker(t)
	proxy := newSwitchableProxy(t)
	config := fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_unreachable"), proxy.endpoint())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkSwarmLeft(api),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-1.reachable", "true"),
			},
			{
				PreConfig:    func() { proxy.on.Store(false) },
				RefreshState: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-1.reachable", "false"),
					resource.TestCheckResourceAttrSet("swarm_cluster.test", "nodes.node-1.node_id"),
				),
			},
			{
				PreConfig:    func() { proxy.on.Store(true) },
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("swarm_cluster.test", "nodes.node-1.reachable", "true"),
			},
		},
	})
}

func TestAccSwarmCluster_noManager(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_invalid"), testEndpoint()),
				ExpectError: regexp.MustCompile(`No manager`),
			},
		},
	})
}

// checkNodeSpec checks the labels and availability of the test Docker's
// node, as Docker reports them.
func checkNodeSpec(api *client.Client, labels map[string]string, availability swarm.NodeAvailability) resource.TestCheckFunc {
	return func(*terraform.State) error {
		info, err := api.Info(context.Background(), client.InfoOptions{})
		if err != nil {
			return err
		}
		node, err := api.NodeInspect(context.Background(), info.Info.Swarm.NodeID, client.NodeInspectOptions{})
		if err != nil {
			return err
		}
		if !maps.Equal(orEmpty(node.Node.Spec.Labels), labels) || node.Node.Spec.Availability != availability {
			return fmt.Errorf("the node has the labels %v and the availability %s, not %v and %s",
				node.Node.Spec.Labels, node.Node.Spec.Availability, labels, availability)
		}
		return nil
	}
}

// checkSwarmLeft checks that the test's Docker left the swarm.
func checkSwarmLeft(api *client.Client) resource.TestCheckFunc {
	return func(*terraform.State) error {
		result, err := api.Info(context.Background(), client.InfoOptions{})
		if err != nil {
			return err
		}
		if state := result.Info.Swarm.LocalNodeState; state != swarm.LocalNodeStateInactive {
			return fmt.Errorf("the Docker is still in the swarm: %s", state)
		}
		return nil
	}
}

// setLabelByHand sets a label on the test Docker's node, as docker node
// update --label-add would.
func setLabelByHand(t *testing.T, api *client.Client, key, value string) {
	t.Helper()
	ctx := context.Background()
	info, err := api.Info(ctx, client.InfoOptions{})
	require.NoError(t, err)
	node, err := api.NodeInspect(ctx, info.Info.Swarm.NodeID, client.NodeInspectOptions{})
	require.NoError(t, err)
	spec := node.Node.Spec
	spec.Labels[key] = value
	_, err = api.NodeUpdate(ctx, node.Node.ID, client.NodeUpdateOptions{Version: node.Node.Version, Spec: spec})
	require.NoError(t, err)
}
