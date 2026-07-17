# OpenTofu's file provisioner distinguishes a null (unset) attribute from an
# empty string. `content = ""` is set-but-empty: OpenTofu uploads an empty file
# to the destination. pulumi-hcl treats an empty-string content as "unset" and
# rejects the config with "exactly one of source or content must be set".
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
}
