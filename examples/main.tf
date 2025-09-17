terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {
  endpoint = "unix:///var/run/docker.sock"
}

resource "swarm_service" "example" {
  name     = "example-service"
  image    = "nginx:latest"
  replicas = 3

  configurable_attribute = "example"
}

data "swarm_service" "example" {
  id = swarm_service.example.id
}