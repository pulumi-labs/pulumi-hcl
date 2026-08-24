terraform {
  required_providers {
    extbase = {
      source  = "pulumi/extbase"
      version = "45.0.0"
    }
    myext = {
      source  = "pulumi/myext"
      version = "2.0.0"
    }
  }
}

// An extension resource (Greeting) and a base-provider resource (Base) used
// together; Greeting lives in the extension's namespace ("myext") and Base
// lives in the base provider's namespace ("extbase").
resource "myext_greeting" "greeting" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "extbase_base" "base" {
  lifecycle {
    create_before_destroy = true
  }
}
output "parameterValue" {
  value = myext_greeting.greeting.parameter_value
}
output "baseValue" {
  value = extbase_base.base.base_value
}
