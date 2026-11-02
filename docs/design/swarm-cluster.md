# Design: one `swarm_cluster` resource for the whole cluster

Status: proposal, for review before any code (step 1 of 3). Version 1.0.0 replaces v0.2's resources with the one described here.

## Why v0.2 is rewritten

v0.2 has a resource per node, `swarm_init` for the first manager and `swarm_join` for every other node. Read against a real cluster, it has these problems:

- **The connection to Docker is opened when the provider is configured.** A machine that is still booting fails `create`; a daemon that is down fails `read`, so every plan and every destroy.
- **A node's role is guessed by splitting its join token.** There is no promotion, demotion, label, availability nor `node rm`: a node removed from the configuration stays in the cluster as a ghost, `Down` on the managers.
- **Destroying `swarm_init` runs `docker swarm leave --force` on the manager**, which takes the cluster down with it.
- **`update` always fails** without asking for a replacement: changing a certificate blocks the resource.
- **The provider's `host`, `cert_path`… are not used by the resources**, and `key_material` is not marked sensitive.

A cluster is one object whose nodes depend on each other: which manager inits, how many managers keep the quorum, which node is drained before it goes. One resource sees all of them, so it can order the changes safely.

## The resource

```hcl
resource "swarm_cluster" "this" {
  nodes = {
    "node-1" = {
      endpoint       = "tcp://10.0.0.11:2376"
      advertise_addr = "10.0.0.11"
      role           = "manager"
    }
    "node-2" = {
      endpoint       = "tcp://10.0.0.12:2376"
      advertise_addr = "10.0.0.12"
      role           = "manager"
      labels         = { zone = "b" }
    }
    "node-3" = {
      endpoint       = "ssh://deploy@10.0.0.13"
      advertise_addr = "10.0.0.13"
      role           = "worker"
      availability   = "drain"
    }
  }

  tls = {                       # default for the nodes' tcp:// endpoints, each node can override it
    ca_pem   = file("ca.pem")
    cert_pem = file("cert.pem")
    key_pem  = file("key.pem")  # sensitive
  }

  init = {                      # used only when the cluster is created
    default_addr_pool = ["10.20.0.0/16"]
    data_path_port    = 4789
  }

  timeouts = { create = "15m", update = "15m" }
}
```

| Attribute | |
|---|---|
| `nodes` | **Required.** The nodes by a name of the configuration's choosing, which is not Docker's hostname. At least one `manager`. |
| `nodes.<name>.endpoint` | **Required.** The node's Docker: `tcp://HOST:PORT` (TLS) or `ssh://USER@HOST`. It may be unknown at plan time, when the machine is created in the same apply. |
| `nodes.<name>.advertise_addr` | The address the other nodes reach this one on. Default: Docker's choice. |
| `nodes.<name>.role` | `manager` or `worker`. Default `worker`. |
| `nodes.<name>.labels` | The node's labels. Authoritative: a label the configuration does not give is removed. |
| `nodes.<name>.availability` | `active`, `pause` or `drain`. Default `active`. |
| `nodes.<name>.tls` | This node's TLS, over `tls`. |
| `tls` | The default TLS of the `tcp://` endpoints: `ca_pem`, `cert_pem`, `key_pem` (sensitive). |
| `init` | Settings of `docker swarm init`: `default_addr_pool`, `subnet_size`, `data_path_port`. Changing them after creation is refused: Docker cannot. |
| `leave_on_destroy` | Whether destroying the resource makes every node leave the swarm. Default `false`: the cluster stays, only Terraform forgets it. |
| `timeouts` | `create`, `read`, `update`, `delete`. How long to wait for daemons that do not answer yet. |

**Read-only:** `cluster_id`, `manager_addresses`, `join_tokens.manager` and `join_tokens.worker` (sensitive), and per node `node_id`, `reachable` and `status`.

The provider block takes nothing: each node carries its own endpoint, so one configuration can manage several clusters.

## Reconciliation

Every create and update runs the same loop, from what Docker says, not from the state:

1. **Find the cluster.** Connect to the nodes, waiting for each until the timeout. A manager that is already in a swarm gives the cluster: it is **adopted**. A cluster that was initialised outside Terraform, by cloud-init on a first machine for instance, needs no import. If no manager is in a swarm, the first manager by name **inits** it with `init`. Managers in two different swarms are an error, never merged.
2. **Join** the nodes that are not in the cluster, managers first, with the tokens read from a manager.
3. **Promote** the nodes that should be managers, then **demote** the managers that should be workers.
4. **Set** labels and availability.
5. **Remove** the nodes the configuration no longer has: drain, wait until their tasks run elsewhere, demote a manager, `swarm leave` on the node if it answers, then `node rm` from a manager.

**The quorum is never lost.** Joins and promotions come before demotions and removals. A change that would leave fewer managers reachable than half the cluster's managers plus one is refused at plan time, with the order that would make it safe: add the new managers in one apply, remove the old ones in the next.

**Read** asks any manager that answers, and records each node as Docker sees it. A node that does not answer is `reachable = false` in the state: the plan shows it, and does not fail. A node that left on its own, or was removed by hand, shows as a change to apply.

**Delete** forgets the cluster. With `leave_on_destroy`, it removes the workers, then the managers but the last, then makes the last one leave.

**Import** takes the endpoint of a manager: `terraform import swarm_cluster.this tcp://10.0.0.11:2376`. The configuration then names the nodes.

## Out of scope

- Services, stacks, configs, secrets and networks: what runs on the cluster belongs to the tool that deploys it.
- Creating the machines: the cloud's provider does, and gives `swarm_cluster` their endpoints.
- Autolock and the rotation of the CA and of the join tokens: possible later, as attributes of the cluster.

## A data source

`data "swarm_cluster"` reads a cluster from one manager's endpoint and exposes the same read-only attributes, for a configuration that only needs the tokens or the managers' addresses.

## Moving from v0.2

v0.2's resources go. A configuration that used them:

1. removes `swarm_init` and `swarm_join` from its state, with `terraform state rm`, which leaves the cluster running;
2. declares `swarm_cluster` with the same nodes: the first apply adopts the running cluster, and changes only what differs.

The upgrade guide will give the commands.

## Next steps

- **Step 2, one node:** init, adoption, read of an unreachable daemon, delete, import. Acceptance tests on the CI runner's Docker.
- **Step 3, several nodes:** join, roles, labels, availability, removal with drain, loss of a node, the quorum check. Acceptance tests in Docker-in-Docker, 3 managers and 1 worker. Release `v1.0.0`.

## To decide at review

1. **`leave_on_destroy` defaults to `false`.** Destroying the resource then keeps the cluster: safer for clusters that run workloads, at the cost of a cluster Terraform no longer knows.
2. **Labels are authoritative.** The alternative is to manage only the keys the configuration gives, and leave the others, which tools may set.
3. **Waiting for a daemon** uses the resource's `timeouts`. The alternative is a separate `connect_timeout` per node.
4. **The node's name** is the configuration's key, not Docker's hostname. Matching Docker's node is done by its endpoint, and then its `node_id`.
