terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

// No provider options here: the providers map must be inherited from the
// enclosing local component and flow through the remote component's
// registration into its construct call.
resource "component_component_foreign_child" "mlc" {
  pulumi {
    name ="${pulumi.module.name}-mlc"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
output "result" {
  value = component_component_foreign_child.mlc.value
}
