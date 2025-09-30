# Terraform Swarm Provider Examples

This directory contains example configurations for the Swarm provider.

## Examples

### [Simple Cluster](simple-cluster/)
A basic example that initializes a single-node swarm cluster using a local Docker socket connection.

**Use case**: Development environments, testing, or single-node deployments.

### [Multi-Node Cluster](multi-node-cluster/)
A complete example that creates a multi-node swarm cluster with multiple managers and workers using SSH connections.

**Use case**: Production environments requiring high availability and fault tolerance.

## Prerequisites

Before running these examples:

1. **Docker Installation**: Ensure Docker is installed and running on all target nodes
2. **Network Connectivity**: Ensure nodes can communicate on port 2377 (swarm management)
3. **SSH Access**: For remote examples, ensure SSH key-based authentication is configured
4. **Firewall Rules**: Open necessary ports for swarm communication:
   - TCP 2377: Cluster management communications
   - TCP/UDP 7946: Communication among nodes
   - UDP 4789: Overlay network traffic

## Running Examples

1. Navigate to an example directory:
   ```bash
   cd examples/simple-cluster
   ```

2. Initialize Terraform:
   ```bash
   terraform init
   ```

3. Review the planned changes:
   ```bash
   terraform plan
   ```

4. Apply the configuration:
   ```bash
   terraform apply
   ```

## Customization

### Network Configuration
Modify the IP addresses and network settings in the examples to match your environment:

```hcl
variable "manager_ips" {
  default = ["10.0.1.10", "10.0.1.11", "10.0.1.12"]
}
```

### SSH Configuration
Customize SSH options for your environment:

```hcl
node {
  host = "ssh://myuser@remote-host"
  ssh_opts = [
    "-o", "StrictHostKeyChecking=no",
    "-i", "/path/to/private/key",
    "-p", "2222"  # Custom SSH port
  ]
}
```

### TLS Configuration
For secure TCP connections:

```hcl
provider "swarm" {
  host      = "tcp://remote-host:2376"
  cert_path = "/path/to/docker/certs"
}
```

## Cleanup

To destroy the created swarm cluster:

```bash
terraform destroy
```

This will safely leave all nodes from the swarm and clean up the cluster.

## Security Considerations

- Use SSH key-based authentication instead of passwords
- Restrict network access to swarm management ports
- Use TLS when connecting to remote Docker daemons
- Rotate swarm join tokens regularly
- Keep Docker and host systems updated

## Troubleshooting

### Connection Issues
- Verify Docker daemon is running: `systemctl status docker`
- Check network connectivity: `telnet <manager-ip> 2377`
- Verify SSH access: `ssh user@host docker info`

### Permission Issues
- Ensure user has Docker permissions: `usermod -aG docker $USER`
- Check SSH key permissions: `chmod 600 ~/.ssh/id_rsa`

### Swarm State Issues
- Check swarm status: `docker info | grep Swarm`
- List nodes: `docker node ls`
- View node details: `docker node inspect <node-id>`