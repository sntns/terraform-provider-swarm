# swarm_join Resource  

The `swarm_join` resource joins a node to an existing Docker Swarm cluster as either a manager or worker node.

## Example Usage

### Join as Worker Node
```hcl
resource "swarm_join" "worker" {
  join_token   = var.worker_token
  remote_addrs = ["192.168.1.100:2377"]
  
  node {
    host = "ssh://root@192.168.1.101"
  }
}
```

### Join as Manager Node
```hcl
resource "swarm_join" "manager" {
  join_token     = var.manager_token
  remote_addrs   = ["192.168.1.100:2377"] 
  advertise_addr = "192.168.1.102"
  listen_addr    = "0.0.0.0:2377"
  
  node {
    host = "ssh://root@192.168.1.102"
    ssh_opts = [
      "-o", "StrictHostKeyChecking=no"
    ]
  }
}
```

### Complete Multi-Node Setup
```hcl
# Initialize swarm on bootstrap node
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  
  node {
    host = "ssh://root@192.168.1.100"
  }
}

# Join manager node
resource "swarm_join" "manager1" {
  join_token     = swarm_init.cluster.manager_token
  remote_addrs   = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.102"
  
  node {
    host = "ssh://root@192.168.1.102"
  }
}

# Join worker nodes
resource "swarm_join" "worker1" {
  join_token   = swarm_init.cluster.worker_token
  remote_addrs = ["192.168.1.100:2377"]
  
  node {
    host = "ssh://root@192.168.1.103"
  }
}

resource "swarm_join" "worker2" {
  join_token   = swarm_init.cluster.worker_token
  remote_addrs = ["192.168.1.100:2377"]
  
  node {
    host = "ssh://root@192.168.1.104"
  }
}
```

## Argument Reference

- `node` (Required, Block) - Docker connection configuration for the node to join
  - `host` (Required) - Docker daemon host (e.g., "unix:///var/run/docker.sock", "tcp://host:2376", "ssh://user@host")
  - `context` (Optional) - Docker context to use
  - `ssh_opts` (Optional) - List of SSH options when using SSH connection
  - `cert_material` (Optional) - PEM-encoded content of Docker client certificate
  - `key_material` (Optional) - PEM-encoded content of Docker client private key
  - `ca_material` (Optional) - PEM-encoded content of Docker CA certificate
  - `cert_path` (Optional) - Path to directory with Docker TLS config files

- `join_token` (Required, Sensitive) - Join token obtained from swarm manager (use worker token for workers, manager token for managers)

- `remote_addrs` (Required) - List of addresses of existing swarm managers (e.g., ["192.168.1.100:2377"])

- `advertise_addr` (Optional) - Externally reachable address advertised to other nodes. If not specified, Docker will choose automatically.

- `listen_addr` (Optional) - Listen address for the raft consensus protocol. Only used for manager nodes. Defaults to "0.0.0.0:2377".

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

- `id` - Terraform resource identifier
- `node_id` - Docker Swarm node ID assigned after joining
- `node_role` - Role of the node in the swarm ("manager" or "worker")

## Import

Swarm join resources can be imported using the node ID:

```shell
terraform import swarm_join.worker <node-id>
```

## Notes

- The node will automatically leave the swarm when this resource is destroyed
- Manager nodes require the manager join token, worker nodes require the worker join token
- Join tokens are sensitive and should be handled securely
- The node role (manager/worker) is determined by the type of join token used
- Network connectivity must exist between the joining node and existing swarm managers on the specified ports (default 2377)