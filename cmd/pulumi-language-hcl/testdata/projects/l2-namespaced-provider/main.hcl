terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
    namespaced = {
      source  = "a-namespace/namespaced"
      version = "16.0.0"
    }
  }
}

resource "component_componentcustomrefoutput" "componentRes" {
  value = "foo-bar-baz"
}
resource "namespaced_resource" "res" {
  value        = true
  resource_ref = component_componentcustomrefoutput.componentRes.ref
}
