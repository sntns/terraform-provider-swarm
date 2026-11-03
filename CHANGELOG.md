# Changelog

## [1.0.0](https://github.com/sntns/terraform-provider-swarm/compare/v0.2.0...v1.0.0) (2026-11-03)


### ⚠ BREAKING CHANGES

* swarm_init and swarm_join are removed, and so are the provider's attributes. Remove them from the state with terraform state rm, which leaves the swarm running, then declare a swarm_cluster with the same

### Features

* join, promote, demote and remove the nodes of a cluster ([2c039c4](https://github.com/sntns/terraform-provider-swarm/commit/2c039c41ce38ebc199ebdba6326e7f86022c536f))
* manage a cluster with one swarm_cluster resource ([ddd6a37](https://github.com/sntns/terraform-provider-swarm/commit/ddd6a370c901fd1908a6d3618bb44cb887e81e6b))
* read a cluster from a manager with the swarm_cluster data source ([0ddbd70](https://github.com/sntns/terraform-provider-swarm/commit/0ddbd70586f155e2f123fe3e12bd20833dcd512a))
