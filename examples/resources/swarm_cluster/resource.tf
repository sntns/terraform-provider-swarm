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

  init = {
    default_addr_pool = ["10.20.0.0/16"]
  }
}
