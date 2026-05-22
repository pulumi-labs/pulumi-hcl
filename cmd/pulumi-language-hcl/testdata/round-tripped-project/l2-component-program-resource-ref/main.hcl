pulumi {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

resource "component_component_custom_ref_output" "component1" {
  value = "foo-bar-baz"
}
resource "component_custom" "custom1" {
  value = component_component_custom_ref_output.component1.value
}
resource "component_custom" "custom2" {
  value = component_component_custom_ref_output.component1.ref.value
}
