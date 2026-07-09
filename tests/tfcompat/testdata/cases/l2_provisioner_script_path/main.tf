variable "host" { type = string }
variable "port" { type = number }
variable "user" { type = string }
variable "password" { type = string }
variable "script" { type = string }

resource "simple_resource" "target" {
  input_one = "a"

  connection {
    type        = "ssh"
    host        = var.host
    port        = var.port
    user        = var.user
    password    = var.password
    timeout     = "30s"
    script_path = "/config/terraform_%RAND%.sh"
  }

  provisioner "remote-exec" {
    script = var.script
  }
}
