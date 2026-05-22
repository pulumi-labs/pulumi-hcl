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
resource "component_custom" "custom1" {
  lifecycle {
    create_before_destroy = true
  }
  value = component_componentcustomrefoutput.component1.value
}
resource "component_custom" "custom2" {
  lifecycle {
    create_before_destroy = true
  }
  value = component_componentcustomrefoutput.component1.ref.value
}
