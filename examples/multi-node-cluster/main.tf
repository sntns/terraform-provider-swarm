terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {
  # Uses default unix socket connection
}

variable "manager_ips" {
  description = "IP addresses of manager nodes"
  type        = list(string)
  default     = ["192.168.1.100", "192.168.1.101", "192.168.1.102"]
}

variable "worker_ips" {
  description = "IP addresses of worker nodes" 
  type        = list(string)
  default     = ["192.168.1.103", "192.168.1.104", "192.168.1.105"]
}

# Initialize swarm on first manager node
resource "swarm_init" "cluster" {
  advertise_addr = var.manager_ips[0]
  listen_addr    = "0.0.0.0:2377"
  
  node {
    host = "ssh://root@${var.manager_ips[0]}"
    ssh_opts = [
      "-o", "StrictHostKeyChecking=no",
      "-o", "UserKnownHostsFile=/dev/null"
    ]
  }
}

# Join additional manager nodes
resource "swarm_join" "managers" {
  count = length(var.manager_ips) - 1
  
  join_token     = swarm_init.cluster.manager_token
  remote_addrs   = ["${var.manager_ips[0]}:2377"]
  advertise_addr = var.manager_ips[count.index + 1]
  listen_addr    = "0.0.0.0:2377"
  
  node {
    host = "ssh://root@${var.manager_ips[count.index + 1]}"
    ssh_opts = [
      "-o", "StrictHostKeyChecking=no",
      "-o", "UserKnownHostsFile=/dev/null"
    ]
  }
}

# Join worker nodes
resource "swarm_join" "workers" {
  count = length(var.worker_ips)
  
  join_token     = swarm_init.cluster.worker_token
  remote_addrs   = ["${var.manager_ips[0]}:2377"]
  advertise_addr = var.worker_ips[count.index]
  
  node {
    host = "ssh://root@${var.worker_ips[count.index]}"
    ssh_opts = [
      "-o", "StrictHostKeyChecking=no", 
      "-o", "UserKnownHostsFile=/dev/null"
    ]
  }
}

# Outputs
output "cluster_id" {
  description = "Swarm cluster ID"
  value       = swarm_init.cluster.id
}

output "manager_nodes" {
  description = "Manager node information"
  value = {
    bootstrap = {
      ip      = var.manager_ips[0]
      node_id = "bootstrap"
    }
    additional = [
      for i, join in swarm_join.managers : {
        ip      = var.manager_ips[i + 1]
        node_id = join.node_id
      }
    ]
  }
}

output "worker_nodes" {
  description = "Worker node information"
  value = [
    for i, join in swarm_join.workers : {
      ip      = var.worker_ips[i]
      node_id = join.node_id
    }
  ]
}