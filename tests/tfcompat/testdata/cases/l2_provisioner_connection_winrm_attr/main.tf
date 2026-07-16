variable "host" { type = string }
variable "port" { type = number }
variable "user" { type = string }
variable "password" { type = string }

# OpenTofu's connection block is validated against the superset schema, which
# includes winrm-only attributes (https/insecure/use_ntlm/cacert). On an ssh
# connection the ssh communicator simply ignores them, so the provisioner runs.
resource "simple_resource" "target" {
  input_one = "a"

  connection {
    type     = "ssh"
    host     = var.host
    port     = var.port
    user     = var.user
    password = var.password
    timeout  = "30s"
    use_ntlm = true
  }

  provisioner "remote-exec" {
    inline = ["echo hello-from-remote-exec"]
  }
}
