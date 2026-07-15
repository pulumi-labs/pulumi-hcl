terraform {
  required_providers {
    myext = {
      source  = "pulumi/myext"
      version = "2.0.0"
    }
  }
}

data "extbase_greet" "invoke_0" {
  name = "Pulumi"
}

// Extension parameterization: the SDK is published as "myext" but the resource
// tokens live in the base provider's namespace ("extbase").
resource "extbase_greeting" "greeting" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "extbase_greeting_component" "greetingComp" {
  lifecycle {
    create_before_destroy = true
  }
}
output "parameterValue" {
  value = extbase_greeting.greeting.parameter_value
}
output "parameterValueFromComponent" {
  value = extbase_greeting_component.greetingComp.parameter_value
}
output "invokeGreeting" {
  value = data.extbase_greet.invoke_0.greeting
}
