# swarm_init Resource

The `swarm_init` resource initializes a Docker Swarm cluster on the specified node. This should be run on the bootstrap/manager node of your swarm cluster.

## Example Usage

### Basic Usage
```hcl
resource "swarm_init" "cluster" {
  node {
    host = "unix:///var/run/docker.sock"
  }
}
```

### With Custom Configuration
```hcl
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
  
  node {
    host = "ssh://root@192.168.1.100"
    ssh_opts = [
      "-o", "StrictHostKeyChecking=no",
      "-i", "/path/to/ssh/key"
    ]
  }
}
```

### Using Tokens in Other Resources
```hcl
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  
  node {
    host = "unix:///var/run/docker.sock"
  }
}

output "manager_token" {
  value     = swarm_init.cluster.manager_token
  sensitive = true
}

output "worker_token" {
  value     = swarm_init.cluster.worker_token
  sensitive = true
}
```

## Argument Reference

- `node` (Required, Block) - Docker connection configuration for the bootstrap node
  - `host` (Required) - Docker daemon host (e.g., "unix:///var/run/docker.sock", "tcp://host:2376", "ssh://user@host")
  - `context` (Optional) - Docker context to use
  - `ssh_opts` (Optional) - List of SSH options when using SSH connection
  - `cert_material` (Optional) - PEM-encoded content of Docker client certificate
  - `key_material` (Optional) - PEM-encoded content of Docker client private key 
  - `ca_material` (Optional) - PEM-encoded content of Docker CA certificate
  - `cert_path` (Optional) - Path to directory with Docker TLS config files

- `advertise_addr` (Optional) - Externally reachable address advertised to other nodes. If not specified, Docker will choose automatically.

- `listen_addr` (Optional) - Listen address for the raft consensus protocol. Defaults to "0.0.0.0:2377".

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

- `id` - The Swarm cluster ID
- `manager_token` - Token for joining additional manager nodes (sensitive)
- `worker_token` - Token for joining worker nodes (sensitive)

## Import

Swarm init resources can be imported using the swarm cluster ID:

```shell
terraform import swarm_init.cluster <cluster-id>
```

## Notes

- This resource should only be used once per swarm cluster
- The swarm will be automatically left and disbanded when this resource is destroyed
- Join tokens are automatically rotated by Docker and will be updated in the state
- If the swarm already exists on the target node, Terraform will import the existing state