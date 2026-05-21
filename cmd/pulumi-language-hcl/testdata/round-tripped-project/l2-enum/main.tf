terraform {
  required_providers {
    enum = {
      source  = "pulumi/enum"
      version = "30.0.0"
    }
  }
}

resource "enum_res" "sink1" {
  int_enum    = 1
  string_enum = "two"
}
resource "enum_mod_res" "sink2" {
  int_enum    = 1
  string_enum = "two"
}
resource "enum_mod_nested_res" "sink3" {
  int_enum    = 1
  string_enum = "two"
}
