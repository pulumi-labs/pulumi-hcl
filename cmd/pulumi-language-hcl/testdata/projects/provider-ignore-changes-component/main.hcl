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

resource "conformance-component_simple" "withIgnoreChanges" {
  lifecycle {
    ignore_changes = [value]
  }
  value = true
}
resource "conformance-component_simple" "withoutIgnoreChanges" {
  value = true
}
resource "simple_resource" "simpleResource" {
  value = false
}
