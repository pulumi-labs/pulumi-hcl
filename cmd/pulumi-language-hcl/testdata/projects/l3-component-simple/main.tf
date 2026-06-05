terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "input" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
module "someComponent" {
  source = "./myComponent"
  input  = simple_resource.input.value
}
output "result" {
  value = module.someComponent.output
}
