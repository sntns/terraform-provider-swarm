# Terraform Provider Swarm

[![Tests](https://github.com/sntns/terraform-provider-swarm/workflows/Tests/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ATests)
[![Release](https://github.com/sntns/terraform-provider-swarm/workflows/Release/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ARelease)

A Terraform provider for managing Docker Swarm resources.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.19

## Building The Provider

1. Clone the repository
2. Enter the repository directory
3. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

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
  endpoint = "unix:///var/run/docker.sock"
}

resource "swarm_service" "example" {
  name     = "example-service"
  image    = "nginx:latest"
  replicas = 3
}

data "swarm_service" "example" {
  id = swarm_service.example.id
}
```

## Development

### Running Tests

```shell
# Run unit tests
make test

# Run acceptance tests (requires Docker Swarm)
make testacc
```

### Building for Release

```shell
# Build for all platforms
make release
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -am 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.
=======
# terraform-provider-swarm

A Terraform provider for managing Docker Swarm clusters. This provider allows you to initialize a Docker Swarm cluster on a bootstrap node and join additional nodes to the cluster.

## Features

- Initialize a Docker Swarm cluster with configurable settings
- Join nodes to an existing Docker Swarm cluster as workers or managers
- Automatically retrieve join tokens for manager and worker nodes
- Support for different Docker daemon configurations

## Usage

### Provider Configuration

```hcl
terraform {
  required_providers {
    swarm = {
      source = "sntns/swarm"
    }
  }
}

provider "swarm" {
  host = "unix:///var/run/docker.sock"  # Optional, defaults to unix:///var/run/docker.sock
  
  # Optional: Multi-node configuration for managing different Docker hosts
  nodes = {
    "bootstrap" = {
      host = "unix:///var/run/docker.sock"
    }
    "worker1" = {
      host      = "tcp://192.168.1.101:2376"
      cert_path = "/path/to/worker1/cert.pem"
      key_path  = "/path/to/worker1/key.pem"
      ca_path   = "/path/to/worker1/ca.pem"
    }
    "manager1" = {
      host      = "tcp://192.168.1.102:2376"
      cert_path = "/path/to/manager1/cert.pem"
      key_path  = "/path/to/manager1/key.pem"
      ca_path   = "/path/to/manager1/ca.pem"
    }
  }
}
```

### Initialize a Swarm Cluster

```hcl
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  listen_addr    = "0.0.0.0:2377"
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

### Join a Node to the Cluster

```hcl
# Join as a worker node
resource "swarm_join" "worker" {
  join_token    = swarm_init.cluster.worker_token
  remote_addrs  = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.101"
}

# Join as a manager node
resource "swarm_join" "manager" {
  join_token    = swarm_init.cluster.manager_token
  remote_addrs  = ["192.168.1.100:2377"]
  advertise_addr = "192.168.1.102"
  listen_addr   = "0.0.0.0:2377"
}
```

## Resources

### `swarm_init`

Initializes a Docker Swarm cluster on the current node.

#### Arguments

- `advertise_addr` (Optional) - Externally reachable address advertised to other nodes
- `listen_addr` (Optional) - Listen address for the raft consensus protocol

#### Attributes

- `id` - Swarm cluster ID
- `manager_token` - Token for joining nodes as managers (sensitive)
- `worker_token` - Token for joining nodes as workers (sensitive)

### `swarm_join`

Joins a node to an existing Docker Swarm cluster.

#### Arguments

- `join_token` (Required, Sensitive) - Join token from the swarm manager
- `remote_addrs` (Required) - List of addresses of existing swarm managers
- `advertise_addr` (Optional) - Externally reachable address advertised to other nodes
- `listen_addr` (Optional) - Listen address for the raft consensus protocol (managers only)

#### Attributes

- `id` - Resource identifier
- `node_id` - ID of the node after joining
- `node_role` - Role of the node (manager or worker)

## Requirements

- Docker must be installed and running on all nodes
- The Docker daemon must be accessible (default: unix:///var/run/docker.sock)
- Network connectivity between swarm nodes

## Implementation Details

This provider uses a hybrid approach combining Docker API with Docker CLI:

- **Docker API**: Used for all core swarm operations (init, join, leave, inspect) for reliable programmatic access
- **Join Token Retrieval**: Join tokens are retrieved directly from SwarmInspect API response (JoinTokens field)
- **Multi-host Support**: Provider can be configured with different hosts, certificates, and connection settings
- **Node Configuration**: Support for mapping multiple node configurations within a single provider block
- **Error Handling**: Comprehensive error handling for network issues, authentication, and swarm state conflicts

## Building

```bash
go build -o terraform-provider-swarm
```

## Testing

The provider uses Docker CLI commands to manage the swarm, so Docker must be available on the system where Terraform runs.

## License

MIT License
