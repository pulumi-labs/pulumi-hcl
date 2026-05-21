terraform {
  required_providers {
    byepackage = {
      source  = "pulumi/byepackage"
      version = "2.0.0"
    }
    hipackage = {
      source  = "pulumi/hipackage"
      version = "2.0.0"
    }
  }
}

// The resource name is based on the parameter value
resource "hipackage_hello_world" "example1" {
}
resource "hipackage_hello_world_component" "exampleComponent1" {
}
// The resource name is based on the parameter value
resource "byepackage_goodbye_world" "example2" {
}
resource "byepackage_goodbye_world_component" "exampleComponent2" {
}
output "parameterValue1" {
  value = hipackage_hello_world.example1.parameter_value
}
output "parameterValueFromComponent1" {
  value = hipackage_hello_world_component.exampleComponent1.parameter_value
}
output "parameterValue2" {
  value = byepackage_goodbye_world.example2.parameter_value
}
output "parameterValueFromComponent2" {
  value = byepackage_goodbye_world_component.exampleComponent2.parameter_value
}
