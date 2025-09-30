# Swarm Provider

The Swarm provider enables Terraform to manage Docker Swarm clusters. It allows you to initialize new swarm clusters and join nodes to existing clusters.

## Requirements

- [Docker](https://docs.docker.com/get-docker/) installed and running
- Network connectivity between swarm nodes
- Appropriate permissions to manage Docker daemon

## Provider Configuration

The provider supports several authentication and connection methods:

### Unix Socket (Default)
```hcl
provider "swarm" {
  # Uses unix:///var/run/docker.sock by default
}
```

### TCP Connection
```hcl
provider "swarm" {
  host = "tcp://localhost:2376"
}
```

### TCP with TLS
```hcl
provider "swarm" {
  host      = "tcp://localhost:2376" 
  cert_path = "/path/to/docker/certs"
}
```

### SSH Connection
```hcl
provider "swarm" {
  host = "ssh://user@remote-host"
}
```

## Configuration Options

- `host` (Optional) - Docker daemon host. Defaults to `unix:///var/run/docker.sock` or `DOCKER_HOST` environment variable
- `cert_path` (Optional) - Path to directory with Docker TLS configuration files (ca.pem, cert.pem, key.pem)
- `key_path` (Optional) - Path to Docker client private key file  
- `ca_path` (Optional) - Path to Docker CA certificate file
- `api_version` (Optional) - Docker API version to use
- `registry_auth` (Optional, Sensitive) - Registry authentication configuration as a map

## Environment Variables

The provider respects the following environment variables:

- `DOCKER_HOST` - Docker daemon host (when `host` is not specified)
- `DOCKER_CERT_PATH` - Path to Docker TLS certificates
- `DOCKER_TLS_VERIFY` - Enable TLS verification

## Resources

- [`swarm_init`](resources/swarm_init.md) - Initialize a Docker Swarm cluster
- [`swarm_join`](resources/swarm_join.md) - Join a node to a Docker Swarm cluster

## Example Usage

```hcl
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

# Initialize swarm on bootstrap node  
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
}

# Join worker node
resource "swarm_join" "worker" {
  join_token    = swarm_init.cluster.worker_token
  remote_addrs  = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.101"
  
  node {
    host = "ssh://root@192.168.1.101"
  }
}
```