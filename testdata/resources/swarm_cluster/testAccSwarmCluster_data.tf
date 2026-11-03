resource "swarm_cluster" "test" {
  nodes = {
    "node-1" = {
      endpoint       = "%[1]s"
      advertise_addr = "127.0.0.1"
      role           = "manager"
      labels         = { zone = "a" }
    }
  }
  leave_on_destroy = true
}

data "swarm_cluster" "test" {
  endpoint   = "%[1]s"
  depends_on = [swarm_cluster.test]
}
