# Terraform Provider Swarm

[![Tests](https://github.com/sntns/terraform-provider-swarm/workflows/Tests/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ATests)
[![Release](https://github.com/sntns/terraform-provider-swarm/workflows/Release/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions?query=workflow%3ARelease)

A Terraform provider for managing Docker Swarm clusters. This provider allows you to initialize Docker Swarm clusters and join nodes to existing clusters using the Terraform Plugin Framework.

## Features

- 🚀 **Initialize Docker Swarm clusters** with configurable settings
- 🔗 **Join nodes** to existing swarm clusters as managers or workers  
- 🔑 **Automatic token management** for secure node joining
- 🌐 **Multiple connection methods** (Unix socket, TCP, SSH)
- 🔒 **TLS support** for secure remote connections
- 📝 **Comprehensive documentation** and examples

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.19 (for building from source)
- [Docker](https://docs.docker.com/get-docker/) installed and running on target nodes

## Quick Start

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

# Initialize swarm cluster
resource "swarm_init" "cluster" {
  advertise_addr = "192.168.1.100"
  
  node {
    host = "unix:///var/run/docker.sock"
  }
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

## Documentation

### Provider Configuration
- [Provider Documentation](docs/provider.md) - Complete provider configuration reference

### Resources
- [`swarm_init`](docs/resources/swarm_init.md) - Initialize a Docker Swarm cluster
- [`swarm_join`](docs/resources/swarm_join.md) - Join nodes to a swarm cluster

### Examples
- [Simple Cluster](examples/simple-cluster/) - Basic single-node setup
- [Multi-Node Cluster](examples/multi-node-cluster/) - Production-ready multi-node setup
- [Examples README](examples/README.md) - Complete examples guide

## Provider Configuration

The provider supports multiple connection methods:

### Unix Socket (Default)
```hcl
provider "swarm" {
  # Uses unix:///var/run/docker.sock by default
}
```

### Remote TCP Connection
```hcl
provider "swarm" {
  host = "tcp://remote-host:2376"
}
```

### SSH Connection
```hcl
provider "swarm" {
  host = "ssh://user@remote-host"
}
```

### TLS Configuration
```hcl
provider "swarm" {
  host      = "tcp://remote-host:2376"
  cert_path = "/path/to/docker/certs"
}
```

## Building The Provider

1. Clone the repository:
   ```bash
   git clone https://github.com/sntns/terraform-provider-swarm.git
   cd terraform-provider-swarm
   ```

2. Build the provider:
   ```bash
   go build -o terraform-provider-swarm
   ```

3. Install locally:
   ```bash
   make install
   ```

## Development

### Prerequisites
- Go 1.19+ installed
- Docker installed and running
- Make (optional, for convenience commands)

### Running Tests
```bash
# Unit tests
go test ./...

# Acceptance tests (requires Docker)
TF_ACC=1 go test ./... -v

# Lint code
golangci-lint run
```

### Project Structure
```
├── internal/
│   ├── provider/     # Provider implementation
│   ├── resources/    # Resource implementations  
│   └── docker/       # Docker client management
├── examples/         # Example configurations
├── docs/            # Documentation
└── test/            # Test configurations
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Workflow
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with tests
4. Run tests and linting (`go test ./...` and `golangci-lint run`)
5. Commit your changes (`git commit -am 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## Architecture

This provider follows Terraform Plugin Framework best practices:

- **Provider**: Handles configuration and Docker client setup
- **Resources**: Implement CRUD operations for swarm management
- **Docker Client**: Abstracted client handling with multiple connection methods
- **Testing**: Comprehensive unit and acceptance test suite
- **Documentation**: Complete user and developer documentation

### Security Features
- TLS 1.2+ for secure connections
- Sensitive data handling for join tokens
- SSH key-based authentication support
- Certificate validation for TLS connections

## Troubleshooting

### Common Issues

**Connection Refused**
- Ensure Docker daemon is running: `systemctl status docker`
- Check firewall settings for ports 2377, 7946, 4789
- Verify network connectivity between nodes

**Permission Denied**  
- Add user to docker group: `usermod -aG docker $USER`
- Check SSH key permissions: `chmod 600 ~/.ssh/id_rsa`

**TLS Verification Failed**
- Verify certificate paths and permissions
- Check that certificates match the hostname
- Use `DOCKER_TLS_VERIFY=1` environment variable if needed

For more troubleshooting help, see the [Examples README](examples/README.md).

## License

This project is licensed under the Mozilla Public License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
- Uses [Docker Go SDK](https://github.com/docker/docker) for swarm management
- Inspired by the Docker and Terraform communities
