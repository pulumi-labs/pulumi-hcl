terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "first-untainted" {
  value = true
}
resource "simple_resource" "first-tainted" {
  value = ! var.input
}
variable "input" {
  type = bool
}
output "untainted" {
  value = simple_resource.first-untainted.value
}
output "tainted" {
  value = simple_resource.first-tainted.value
}
