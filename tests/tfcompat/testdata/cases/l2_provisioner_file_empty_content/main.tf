# OpenTofu's provisioners distinguish a null (unset) attribute from a set-but-
# empty value. `content = ""` on a file provisioner is set: OpenTofu uploads an
# empty file to the destination. Likewise `scripts = []` on a remote-exec
# provisioner is set and apply succeeds, running nothing. pulumi-hcl treated
# both as "unset" and rejected the config with "exactly one of ... must be
# set". (`inline = []` is set but fails in OpenTofu — the generated empty
# script cannot be uploaded — so it stays an apply error and is out of this
# case.)
variable "host" { type = string }
variable "port" { type = number }
variable "user" { type = string }
variable "password" { type = string }

resource "simple_resource" "target" {
  input_one = "a"

  connection {
    type     = "ssh"
    host     = var.host
    port     = var.port
    user     = var.user
    password = var.password
    timeout  = "30s"
  }

  provisioner "file" {
    content     = ""
    destination = "/config/empty-from-content.txt"
  }

  provisioner "remote-exec" {
    scripts = []
  }
}
