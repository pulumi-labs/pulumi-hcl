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

resource "hipackage_helloworld" "example1" {
}
resource "hipackage_helloworldcomponent" "exampleComponent1" {
}
resource "byepackage_goodbyeworld" "example2" {
}
resource "byepackage_goodbyeworldcomponent" "exampleComponent2" {
}
output "parameterValue1" {
  value = hipackage_helloworld.example1.parameter_value
}
output "parameterValueFromComponent1" {
  value = hipackage_helloworldcomponent.exampleComponent1.parameter_value
}
output "parameterValue2" {
  value = byepackage_goodbyeworld.example2.parameter_value
}
output "parameterValueFromComponent2" {
  value = byepackage_goodbyeworldcomponent.exampleComponent2.parameter_value
}
