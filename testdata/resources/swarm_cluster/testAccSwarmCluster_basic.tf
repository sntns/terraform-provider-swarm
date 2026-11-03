resource "swarm_cluster" "test" {
  nodes = {
    "node-1" = {
      endpoint       = "%[1]s"
      advertise_addr = "127.0.0.1"
      role           = "manager"
      labels         = %[2]s
      availability   = "%[3]s"
    }
  }
  init = {
    data_path_port = %[4]d
  }
  leave_on_destroy = true
}
