terraform {
  required_providers {
    myext = {
      source  = "pulumi/myext"
      version = "2.0.0"
    }
  }
}

data "myext_greet" "invoke_0" {
  name = "Pulumi"
}

// Extension parameterization: the SDK is published as "myext" and the resource
// tokens live in the extension's own namespace.
resource "myext_greeting" "greeting" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "myext_greeting_component" "greetingComp" {
  lifecycle {
    create_before_destroy = true
  }
}
output "parameterValue" {
  value = myext_greeting.greeting.parameter_value
}
output "parameterValueFromComponent" {
  value = myext_greeting_component.greetingComp.parameter_value
}
output "invokeGreeting" {
  value = data.myext_greet.invoke_0.greeting
}
