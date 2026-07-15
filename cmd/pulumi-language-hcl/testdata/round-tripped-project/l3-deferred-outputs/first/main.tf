terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "first-untainted" {
  pulumi {
    name ="${pulumi.module.name}-first-untainted"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "first-tainted" {
  pulumi {
    name ="${pulumi.module.name}-first-tainted"
  }
  lifecycle {
    create_before_destroy = true
  }
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
