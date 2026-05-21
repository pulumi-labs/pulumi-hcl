terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "res" {
  boolean      = true
  float        = 3.14
  integer      = 42
  string       = "hello"
  number_array = [-1, 0, 1]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
