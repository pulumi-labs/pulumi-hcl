terraform {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

resource "subpackage_helloworld" "example" {
}
resource "subpackage_helloworldcomponent" "exampleComponent" {
}
output "parameterValue" {
  value = subpackage_helloworld.example.parameter_value
}
output "parameterValueFromComponent" {
  value = subpackage_helloworldcomponent.exampleComponent.parameter_value
}
