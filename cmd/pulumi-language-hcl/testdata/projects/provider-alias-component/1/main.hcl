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

resource "simple_resource" "parent" {
  value = true
}
resource "conformance-component_simple" "res" {
  parent = simple_resource.parent
  aliases = [{
    no_parent = true
  }]
  value = true
}
resource "simple_resource" "simpleResource" {
  value = false
}
