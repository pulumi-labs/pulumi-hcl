terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

data "component_identity" "invoke_0" {
  input      = "reachable"
  depends_on = [component_component_custom_ref_output.target]
}

resource "component_component_custom_ref_output" "target" {
  lifecycle {
    create_before_destroy = true
  }
  value = "checked"
}
output "echoed" {
  value = data.component_identity.invoke_0.result
}
