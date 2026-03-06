terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

resource "component_componentcustomrefoutput" "component1" {
  value = "foo-bar-baz"
}
resource "component_componentcustomrefinputoutput" "component2" {
  input_ref = component_componentcustomrefoutput.component1.ref
}
resource "component_custom" "custom1" {
  value = component_componentcustomrefinputoutput.component2.input_ref.value
}
resource "component_custom" "custom2" {
  value = component_componentcustomrefinputoutput.component2.output_ref.value
}
