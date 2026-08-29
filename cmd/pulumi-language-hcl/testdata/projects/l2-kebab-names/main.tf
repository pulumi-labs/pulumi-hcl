terraform {
  required_providers {
    kebab-names = {
      source  = "pulumi/kebab-names"
      version = "52.0.0"
    }
  }
}

data "kebab-names_kebab-module_do-something" "invoke_0" {
  the-input = "hello"
  nested = {
    value = "nested"
  }
}

// The package name, module name, resource names, object type names and property names are all
// kebab-case.
resource "kebab-names_kebab-module_some-resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  the-input = true
  nested = {
    nested-value = "nested"
  }
}
resource "kebab-names_kebab-module_another-resource" "second" {
  lifecycle {
    create_before_destroy = true
  }
  the-input = kebab-names_kebab-module_some-resource.first.the-output.nested-output
}
// Whole objects in stack outputs keep their wire-format keys
output "theOutput" {
  value = kebab-names_kebab-module_some-resource.first.the-output
}
// The function name and its argument and result property names are kebab-case. The nested object
// type carries a property with a schema default value.
output "invoked" {
  value = data.kebab-names_kebab-module_do-something.invoke_0.the-output
}
