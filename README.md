# terraform-provider-swarm

[![Terraform Registry](https://img.shields.io/badge/terraform-sntns%2Fswarm-7B42BC?logo=terraform)](https://registry.terraform.io/providers/sntns/swarm)
[![Latest release](https://img.shields.io/github/v/release/sntns/terraform-provider-swarm?sort=semver)](https://github.com/sntns/terraform-provider-swarm/releases/latest)
[![Release](https://github.com/sntns/terraform-provider-swarm/actions/workflows/release.yml/badge.svg)](https://github.com/sntns/terraform-provider-swarm/actions/workflows/release.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Manages Docker Swarm clusters from Terraform or OpenTofu. A `swarm_cluster` inits its cluster, or adopts one that runs already, and keeps its nodes as the configuration gives them: role, labels and availability. Each node gives its own Docker endpoint, so the provider takes no configuration.

```hcl
terraform {
  required_providers {
    swarm = { source = "sntns/swarm" }
  }
}

resource "swarm_cluster" "this" {
  nodes = {
    "node-1" = {
      endpoint       = "tcp://10.0.0.11:2376"
      advertise_addr = "10.0.0.11"
      role           = "manager"
      labels         = { zone = "a" }
    }
  }

  tls = {
    ca_pem   = file("ca.pem")
    cert_pem = file("cert.pem")
    key_pem  = file("key.pem")
  }
}
```

## Documentation

On the [Terraform Registry](https://registry.terraform.io/providers/sntns/swarm/latest/docs):

- [The provider](docs/index.md): what it manages, what it needs, compatibility.
- [`swarm_cluster`](docs/resources/cluster.md): every setting of a cluster and its nodes.
- [The `swarm_cluster` data source](docs/data-sources/cluster.md): a cluster's tokens and nodes, read from a manager.
- [The design](docs/design/swarm-cluster.md): how a cluster is reconciled, and why.

## Status

`main` is on its way to 1.0.0, which replaces v0.2's `swarm_init` and `swarm_join` with `swarm_cluster`. It manages clusters of one node for now: joining, promoting and removing nodes come next.

## Development

```sh
make            # lint and unit tests
make generate   # the docs, from the schema, templates/ and examples/
make testacc    # acceptance tests
```

The acceptance tests init and leave swarms on the Docker of `DOCKER_HOST`, else the local socket, and refuse a Docker that is in a swarm already. On a machine whose Docker runs one, give them a throwaway daemon:

```sh
docker run -d --privileged --name swarm-acc -e DOCKER_TLS_CERTDIR= -p 127.0.0.1:23750:2375 docker:29-dind
DOCKER_HOST=tcp://127.0.0.1:23750 make testacc
```
