terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {}

# This would be used for testing when Docker is available
# resource "swarm_init" "test" {
#   advertise_addr = "127.0.0.1"
# }