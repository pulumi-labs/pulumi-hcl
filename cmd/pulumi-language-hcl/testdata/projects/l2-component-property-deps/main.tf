terraform {
  required_providers {
    component-property-deps = {
      source  = "pulumi/component-property-deps"
      version = "1.33.7"
    }
  }
}

call "component1" "refs" {
  resource      = component-property-deps_custom.custom1
  resource_list = [component-property-deps_custom.custom1, component-property-deps_custom.custom2]
  resource_map = {
    "one" = component-property-deps_custom.custom1
    "two" = component-property-deps_custom.custom2
  }
}

resource "component-property-deps_custom" "custom1" {
  lifecycle {
    create_before_destroy = true
  }
  value = "hello"
}
resource "component-property-deps_custom" "custom2" {
  lifecycle {
    create_before_destroy = true
  }
  value = "world"
}
resource "component-property-deps_component" "component1" {
  lifecycle {
    create_before_destroy = true
  }
  resource      = component-property-deps_custom.custom1
  resource_list = [component-property-deps_custom.custom1, component-property-deps_custom.custom2]
  resource_map = {
    "one" = component-property-deps_custom.custom1
    "two" = component-property-deps_custom.custom2
  }
}
output "propertyDepsFromCall" {
  value = call.component1.refs.result
}
