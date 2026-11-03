resource "swarm_cluster" "this" {
  nodes = {
    "node-1" = {
      endpoint       = "tcp://10.0.0.11:2376"
      advertise_addr = "10.0.0.11"
      role           = "manager"
      labels         = { zone = "a" }
    }
    "node-2" = {
      endpoint       = "tcp://10.0.0.12:2376"
      advertise_addr = "10.0.0.12"
      role           = "manager"
      labels         = { zone = "b" }
    }
    "node-3" = {
      endpoint       = "tcp://10.0.0.13:2376"
      advertise_addr = "10.0.0.13"
      role           = "manager"
      labels         = { zone = "c" }
    }
    "node-4" = {
      endpoint     = "ssh://deploy@10.0.0.14"
      role         = "worker"
      availability = "drain"
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
