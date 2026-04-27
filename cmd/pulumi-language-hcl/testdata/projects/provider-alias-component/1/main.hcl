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

// Make a simple resource to use as a parent
resource "simple_resource" "parent" {
  value = true
}
// parent "res" to a new parent and alias it so it doesn't recreate.
resource "conformance-component_simple" "res" {
  parent = simple_resource.parent
  aliases = [{
    no_parent = true
  }]
  value = true
}
// Make a simple resource so that plugin detection works.
resource "simple_resource" "simpleResource" {
  value = false
}
