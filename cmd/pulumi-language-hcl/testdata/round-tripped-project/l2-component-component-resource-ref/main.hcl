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
resource "component_component_custom_ref_input_output" "component2" {
  input_ref = component_component_custom_ref_output.component1.ref
}
resource "component_custom" "custom1" {
  value = component_component_custom_ref_input_output.component2.input_ref.value
}
resource "component_custom" "custom2" {
  value = component_component_custom_ref_input_output.component2.output_ref.value
}
