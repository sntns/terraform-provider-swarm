package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccServiceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccServiceResourceConfig("example"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("swarm_service.test", "name", "example"),
					resource.TestCheckResourceAttr("swarm_service.test", "image", "nginx:latest"),
					resource.TestCheckResourceAttr("swarm_service.test", "configurable_attribute", "example"),
					resource.TestCheckResourceAttrSet("swarm_service.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "swarm_service.test",
				ImportState:       true,
				ImportStateVerify: true,
				// This is not normally necessary, but is here because this
				// example code does not have an actual upstream service.
				// Once the Create method is able to refresh information from
				// the upstream service, this can be removed.
				ImportStateVerifyIgnore: []string{"configurable_attribute"},
			},
			// Update and Read testing
			{
				Config: testAccServiceResourceConfig("updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("swarm_service.test", "configurable_attribute", "updated"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccServiceResourceConfig(configurableAttribute string) string {
	return fmt.Sprintf(`
resource "swarm_service" "test" {
  name     = "example"
  image    = "nginx:latest"
  replicas = 1
  configurable_attribute = %[1]q
}
`, configurableAttribute)
}