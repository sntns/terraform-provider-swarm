package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
)

func TestSwarmInitResource_Metadata(t *testing.T) {
	r := NewSwarmInitResource()
	
	req := resource.MetadataRequest{
		ProviderTypeName: "swarm",
	}
	resp := &resource.MetadataResponse{}
	
	r.Metadata(context.Background(), req, resp)
	
	assert.Equal(t, "swarm_init", resp.TypeName)
}

func TestSwarmInitResource_Schema(t *testing.T) {
	r := NewSwarmInitResource()
	
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	
	r.Schema(context.Background(), req, resp)
	
	assert.NotNil(t, resp.Schema)
	assert.Equal(t, "Initialize a Docker Swarm cluster.", resp.Schema.Description)
	
	// Check required attributes exist
	assert.Contains(t, resp.Schema.Attributes, "id")
	assert.Contains(t, resp.Schema.Attributes, "node")
	assert.Contains(t, resp.Schema.Attributes, "advertise_addr")
	assert.Contains(t, resp.Schema.Attributes, "listen_addr")
	assert.Contains(t, resp.Schema.Attributes, "manager_token")
	assert.Contains(t, resp.Schema.Attributes, "worker_token")
	
	// Verify sensitive attributes
	managerToken := resp.Schema.Attributes["manager_token"]
	workerToken := resp.Schema.Attributes["worker_token"]
	assert.True(t, managerToken.(interface{ IsSensitive() bool }).IsSensitive())
	assert.True(t, workerToken.(interface{ IsSensitive() bool }).IsSensitive())
}

func TestSwarmInitResource_Configure(t *testing.T) {
	r := NewSwarmInitResource().(resource.ResourceWithConfigure)
	
	req := resource.ConfigureRequest{}
	resp := &resource.ConfigureResponse{}
	
	// Should not return any errors for empty configuration
	r.Configure(context.Background(), req, resp)
	
	assert.False(t, resp.Diagnostics.HasError())
}

// Note: Create, Read, Update, Delete tests would require Docker daemon
// These should be integration tests rather than unit tests
func TestSwarmInitResource_InterfaceCompliance(t *testing.T) {
	var _ resource.Resource = &swarmInitResource{}
	var _ resource.ResourceWithConfigure = &swarmInitResource{}
}