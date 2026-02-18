terraform {
  required_providers {
    enum = {
      source  = "pulumi/enum"
      version = "30.0.0"
    }
  }
}

resource "enum_res" "sink1" {
  intEnum    = 1
  stringEnum = "two"
}
resource "enum_mod_res" "sink2" {
  intEnum    = 1
  stringEnum = "two"
}
resource "enum_mod/nested_res" "sink3" {
  intEnum    = 1
  stringEnum = "two"
}
