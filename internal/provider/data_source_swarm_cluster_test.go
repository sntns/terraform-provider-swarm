package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/assert"
)

func TestSwarmClusterDataSource_schema(t *testing.T) {
	t.Parallel()
	var resp datasource.SchemaResponse
	NewSwarmClusterDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	tokens := resp.Schema.Attributes["join_tokens"].(schema.SingleNestedAttribute).Attributes
	assert.True(t, tokens["manager"].IsSensitive() && tokens["worker"].IsSensitive(), "a token joins the cluster")
}

// The data source reads the cluster the resource inited, from its manager.
func TestAccSwarmClusterDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkSwarmLeft(testDocker(t)),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_data"), testEndpoint()),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.swarm_cluster.test", "id", "swarm_cluster.test", "id"),
					resource.TestCheckResourceAttrPair("data.swarm_cluster.test", "join_tokens.worker", "swarm_cluster.test", "join_tokens.worker"),
					resource.TestCheckResourceAttrPair("data.swarm_cluster.test", "nodes.0.node_id", "swarm_cluster.test", "nodes.node-1.node_id"),
					resource.TestCheckResourceAttr("data.swarm_cluster.test", "nodes.#", "1"),
					resource.TestCheckResourceAttr("data.swarm_cluster.test", "nodes.0.role", "manager"),
					resource.TestCheckResourceAttr("data.swarm_cluster.test", "nodes.0.labels.zone", "a"),
					resource.TestCheckResourceAttr("data.swarm_cluster.test", "init.data_path_port", "4789"),
				),
			},
		},
	})
}

func TestAccSwarmClusterDataSource_notManager(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(loadTestConfiguration(t, "swarm_cluster", "testAccSwarmCluster_dataNotManager"), testEndpoint()),
				ExpectError: regexp.MustCompile(`Not a manager`),
			},
		},
	})
}
