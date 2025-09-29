terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {
  host = "unix:///var/run/docker.sock"
}

# Initialize swarm cluster
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
  
  node {
    host = "unix:///var/run/docker.sock"
  }
}

# Output tokens for manual use or other configurations
output "manager_token" {
  description = "Token for joining manager nodes"
  value       = swarm_init.cluster.manager_token
  sensitive   = true
}

output "worker_token" {
  description = "Token for joining worker nodes"
  value       = swarm_init.cluster.worker_token
  sensitive   = true
}

output "cluster_id" {
  description = "Swarm cluster ID"
  value       = swarm_init.cluster.id
}