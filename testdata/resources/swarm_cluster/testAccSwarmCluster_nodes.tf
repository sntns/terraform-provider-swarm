resource "swarm_cluster" "test" {
  nodes = {
%[1]s
  }
  leave_on_destroy = true
  timeouts         = { create = "5m", update = "5m", delete = "5m" }
}
