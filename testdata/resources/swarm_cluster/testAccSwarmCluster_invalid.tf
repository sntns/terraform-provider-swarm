resource "swarm_cluster" "test" {
  nodes = {
    "node-1" = {
      endpoint = "%[1]s"
    }
  }
}
