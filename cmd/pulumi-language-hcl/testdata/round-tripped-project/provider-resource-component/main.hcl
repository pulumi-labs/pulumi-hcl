terraform {
  required_providers {
    conformance_component = {
      source  = "pulumi/conformance-component"
      version = "22.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "conformance_component_simple" "res" {
  value = true
}
resource "simple_resource" "simpleResource" {
  value = false
}
