terraform {
  required_providers {
    kebab-names = {
      source  = "pulumi/kebab-names"
      version = "52.0.0"
    }
  }
}

// The package name and module name are kebab-case. Resource and object type names cannot be
// kebab-case yet (the metaschema forbids hyphens in the member segment of a token), and kebab-case
// property names are not yet handled by all code generators.
resource "kebab-names_kebab-module_some_resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = true
  nested = {
    nested_value = "nested"
  }
}
resource "kebab-names_kebab-module_another_resource" "second" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = kebab-names_kebab-module_some_resource.first.the_output.nested_output
}
