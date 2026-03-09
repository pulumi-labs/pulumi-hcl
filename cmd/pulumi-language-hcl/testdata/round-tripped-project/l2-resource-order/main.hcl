terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "res2" {
  value = local.localVar
}
resource "simple_resource" "res1" {
  value = true
}
output "out" {
  value = simple_resource.res2.value
}
locals {
  localVar = simple_resource.res1.value
}
