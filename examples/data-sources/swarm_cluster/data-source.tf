# A cluster's tokens and managers, read from one of its managers: to join machines the configuration does not manage.
data "swarm_cluster" "this" {
  endpoint = "ssh://deploy@10.0.0.11"
}

output "worker_join" {
  value     = "docker swarm join --token ${data.swarm_cluster.this.join_tokens.worker} ${data.swarm_cluster.this.manager_addresses[0]}"
  sensitive = true
}
