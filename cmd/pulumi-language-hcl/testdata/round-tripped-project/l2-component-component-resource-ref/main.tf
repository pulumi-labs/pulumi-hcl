terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

resource "component_componentcustomrefoutput" "component1" {
  lifecycle {
    create_before_destroy = true
  }
  value = "foo-bar-baz"
}
resource "component_componentcustomrefinputoutput" "component2" {
  lifecycle {
    create_before_destroy = true
  }
  input_ref = component_componentcustomrefoutput.component1.ref
}
resource "component_custom" "custom1" {
  lifecycle {
    create_before_destroy = true
  }
  value = component_componentcustomrefinputoutput.component2.input_ref.value
}
resource "component_custom" "custom2" {
  lifecycle {
    create_before_destroy = true
  }
  value = component_componentcustomrefinputoutput.component2.output_ref.value
}
