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
  lifecycle {
    replace_triggered_by  = ["trigger-value"]
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "simpleResource" {
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
