terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "second-untainted" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "second-tainted" {
  lifecycle {
    create_before_destroy = true
  }
  value = ! var.input
}
variable "input" {
  type = bool
}
output "untainted" {
  value = simple_resource.second-untainted.value
}
output "tainted" {
  value = simple_resource.second-tainted.value
}
