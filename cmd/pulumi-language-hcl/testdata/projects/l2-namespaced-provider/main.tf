terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
    namespaced = {
      source  = "pulumi/a-namespace/namespaced"
      version = "16.0.0"
    }
  }
}

resource "component_componentcustomrefoutput" "componentRes" {
  lifecycle {
    create_before_destroy = true
  }
  value = "foo-bar-baz"
}
resource "namespaced_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  value        = true
  resource_ref = component_componentcustomrefoutput.componentRes.ref
}
