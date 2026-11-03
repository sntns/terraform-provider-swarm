---
page_title: "Upgrading from v0.2"
subcategory: "Guides"
description: |-
  Replace v0.2's swarm_init and swarm_join with one swarm_cluster, without touching the running swarm.
---

# Upgrading from v0.2

Version 1.0 replaces v0.2's `swarm_init` and `swarm_join`, one resource per node, with one [`swarm_cluster`](../resources/cluster) for the whole cluster, and the provider block takes no configuration any more. The running swarm does not change: Terraform forgets the old resources, then the first apply of `swarm_cluster` adopts the swarm its managers are in.

## 1. Forget the old resources

Remove them from the state, which leaves the swarm running:

```shell
terraform state rm swarm_init.cluster swarm_join.manager1 swarm_join.worker
```

With Terraform 1.7 or later, `removed` blocks do the same in the configuration, applied with the next step:

```terraform
removed {
  from = swarm_init.cluster
  lifecycle {
    destroy = false
  }
}
```

## 2. Declare the cluster

Remove `swarm_init`, `swarm_join` and the provider's attributes (`host`, `cert_path`…), and declare the same nodes in one `swarm_cluster`. Each v0.2 `node { host = … }` becomes a node's `endpoint`, and the managers' `swarm_join` become `role = "manager"`:

```terraform
resource "swarm_cluster" "this" {
  nodes = {
    "node-1" = { endpoint = "ssh://root@192.168.1.100", advertise_addr = "192.168.1.100", role = "manager" }
    "node-2" = { endpoint = "ssh://root@192.168.1.102", advertise_addr = "192.168.1.102", role = "manager" }
    "node-3" = { endpoint = "ssh://root@192.168.1.101", role = "worker" }
  }
}
```

The references change with it:

| v0.2 | 1.0 |
|---|---|
| `swarm_init.cluster.id` | `swarm_cluster.this.id` |
| `swarm_init.cluster.manager_token` | `swarm_cluster.this.join_tokens.manager` |
| `swarm_init.cluster.worker_token` | `swarm_cluster.this.join_tokens.worker` |

## 3. Apply

The plan creates `swarm_cluster.this`. Its apply reaches the nodes, finds the swarm their managers are in and adopts it, without init nor join; it changes only what differs from the configuration. Labels are authoritative: a label set by hand that the configuration does not give is removed, so give the labels the nodes have.

`leave_on_destroy` defaults to `false`: unlike v0.2's `swarm_init`, destroying the resource keeps the swarm running.
