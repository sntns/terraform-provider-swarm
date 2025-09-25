terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

# Configure the Swarm Provider for the bootstrap node
provider "swarm" {
  alias = "bootstrap"
  host  = "tcp://192.168.1.100:2376"
}

# Configure the Swarm Provider for worker node 1
provider "swarm" {
  alias = "worker1"
  host  = "tcp://192.168.1.101:2376"
}

# Configure the Swarm Provider for manager node 1
provider "swarm" {
  alias = "manager1"
  host  = "tcp://192.168.1.102:2376"
}

# Initialize the swarm cluster on the bootstrap node
resource "swarm_init" "cluster" {
  provider       = swarm.bootstrap
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
}

# Join worker node to the cluster
resource "swarm_join" "worker1" {
  provider       = swarm.worker1
  join_token     = swarm_init.cluster.worker_token
  remote_addrs   = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.101"
}

# Join manager node to the cluster
resource "swarm_join" "manager1" {
  provider       = swarm.manager1
  join_token     = swarm_init.cluster.manager_token
  remote_addrs   = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.102"
  listen_addr    = "0.0.0.0:2377"
}

# Outputs
output "cluster_info" {
  value = {
    cluster_id    = swarm_init.cluster.id
    bootstrap_node = "192.168.1.100"
    worker_nodes  = [swarm_join.worker1.node_id]
    manager_nodes = [swarm_join.manager1.node_id]
  }
}

output "join_tokens" {
  description = "Join tokens for the cluster"
  value = {
    manager = swarm_init.cluster.manager_token
    worker  = swarm_init.cluster.worker_token
  }
  sensitive = true
}