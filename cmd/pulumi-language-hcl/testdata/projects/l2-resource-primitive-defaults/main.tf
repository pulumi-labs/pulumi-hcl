terraform {
  required_providers {
    primitive-defaults = {
      source  = "pulumi/primitive-defaults"
      version = "8.0.0"
    }
  }
}

resource "primitive-defaults_resource" "resExplicit" {
  boolean = true
  float   = 3.14
  integer = 42
  string  = "hello"
}
resource "primitive-defaults_resource" "resDefaulted" {
}
