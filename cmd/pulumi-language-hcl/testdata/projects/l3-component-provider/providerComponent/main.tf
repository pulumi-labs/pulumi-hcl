terraform {
  required_providers {
    config = {
      source  = "pulumi/config"
      version = "9.0.0"
    }
  }
}

provider "config" {
  alias = "prov"
  name  = "my config"
}
resource "config_resource" "res" {
  provider = config.prov
  lifecycle {
    create_before_destroy = true
  }
  text = var.text
}
variable "text" {
  type = string
}
output "result" {
  value = config_resource.res.text
}
