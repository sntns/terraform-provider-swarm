resource "swarm_cluster" "test" {
  nodes = {
    "node-1" = {
      endpoint       = "%[1]s"
      advertise_addr = "127.0.0.1"
      role           = "manager"
    }
  }
  leave_on_destroy = true
}
