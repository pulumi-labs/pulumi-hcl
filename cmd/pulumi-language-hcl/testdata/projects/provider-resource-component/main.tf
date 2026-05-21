terraform {
  required_providers {
    conformance-component = {
      source  = "pulumi/conformance-component"
      version = "22.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "conformance-component_simple" "res" {
  value = true
}
// Make a simple resource so that plugin detection works.
resource "simple_resource" "simpleResource" {
  value = false
}
