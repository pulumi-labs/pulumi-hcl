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
    ignore_changes        = [value]
    create_before_destroy = true
  }
  value = true
}
resource "conformance-component_simple" "withoutIgnoreChanges" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
// Make a simple resource so that plugin detection works.
resource "simple_resource" "simpleResource" {
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
