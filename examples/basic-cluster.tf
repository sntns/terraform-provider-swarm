terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

# Configure the Swarm Provider
provider "swarm" {
  host = "unix:///var/run/docker.sock"
}

# Initialize the swarm cluster on the bootstrap node
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
}

# Output the join tokens for use by other nodes
output "manager_token" {
  description = "Token for joining nodes as managers"
  value       = swarm_init.cluster.manager_token
  sensitive   = true
}

output "worker_token" {
  description = "Token for joining nodes as workers"
  value       = swarm_init.cluster.worker_token
  sensitive   = true
}

output "cluster_id" {
  description = "Swarm cluster ID"
  value       = swarm_init.cluster.id
}