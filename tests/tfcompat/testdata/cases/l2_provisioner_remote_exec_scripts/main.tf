variable "host" { type = string }
variable "port" { type = number }
variable "user" { type = string }
variable "password" { type = string }
variable "script_first" { type = string }
variable "script_second" { type = string }

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

  provisioner "remote-exec" {
    scripts = [var.script_first, var.script_second]
  }
}
