terraform {
  required_providers {
    plaincomponent = {
      source  = "pulumi/plaincomponent"
      version = "36.0.0"
    }
  }
}

resource "plaincomponent_component" "myComponent" {
  name = "my-resource"
  settings = {
    enabled = true
    tags = {
      "env" = "test"
    }
  }
}
output "label" {
  value = plaincomponent_component.myComponent.label
}
