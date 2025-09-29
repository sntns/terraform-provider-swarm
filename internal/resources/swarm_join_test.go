package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
)

func TestSwarmJoinResource_Metadata(t *testing.T) {
	r := NewSwarmJoinResource()
	
	req := resource.MetadataRequest{
		ProviderTypeName: "swarm",
	}
	resp := &resource.MetadataResponse{}
	
	r.Metadata(context.Background(), req, resp)
	
	assert.Equal(t, "swarm_join", resp.TypeName)
}

func TestSwarmJoinResource_Schema(t *testing.T) {
	r := NewSwarmJoinResource()
	
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	
	r.Schema(context.Background(), req, resp)
	
	assert.NotNil(t, resp.Schema)
	assert.Equal(t, "Join a node to an existing Docker Swarm cluster.", resp.Schema.Description)
	
	// Check required attributes exist
	assert.Contains(t, resp.Schema.Attributes, "id")
	assert.Contains(t, resp.Schema.Attributes, "node")
	assert.Contains(t, resp.Schema.Attributes, "join_token")
	assert.Contains(t, resp.Schema.Attributes, "remote_addrs")
	assert.Contains(t, resp.Schema.Attributes, "advertise_addr")
	assert.Contains(t, resp.Schema.Attributes, "listen_addr")
	assert.Contains(t, resp.Schema.Attributes, "node_id")
	assert.Contains(t, resp.Schema.Attributes, "node_role")
	
	// Verify sensitive attributes
	joinToken := resp.Schema.Attributes["join_token"]
	assert.True(t, joinToken.(interface{ IsSensitive() bool }).IsSensitive())
}

func TestSwarmJoinResource_Configure(t *testing.T) {
	r := NewSwarmJoinResource().(resource.ResourceWithConfigure)
	
	req := resource.ConfigureRequest{}
	resp := &resource.ConfigureResponse{}
	
	// Should not return any errors for empty configuration
	r.Configure(context.Background(), req, resp)
	
	assert.False(t, resp.Diagnostics.HasError())
}

func TestSwarmJoinResource_InterfaceCompliance(t *testing.T) {
	var _ resource.Resource = &swarmJoinResource{}
	var _ resource.ResourceWithConfigure = &swarmJoinResource{}
}