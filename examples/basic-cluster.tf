terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

# Configure the Swarm Provider for different hosts
# You can configure different providers for different Docker hosts
provider "swarm" {
  host = "unix:///var/run/docker.sock"  # Local host
}

# Example of provider for remote host with TCP connection
provider "swarm" {
  alias    = "remote"
  host     = "tcp://192.168.1.100:2376"
  cert_path = "/path/to/cert.pem"
  key_path  = "/path/to/key.pem"
  ca_path   = "/path/to/ca.pem"
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