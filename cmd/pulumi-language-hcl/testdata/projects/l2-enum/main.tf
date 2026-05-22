terraform {
  required_providers {
    enum = {
      source  = "pulumi/enum"
      version = "30.0.0"
    }
  }
}

resource "enum_res" "sink1" {
  lifecycle {
    create_before_destroy = true
  }
  int_enum    = 1
  string_enum = "two"
}
resource "enum_mod_res" "sink2" {
  lifecycle {
    create_before_destroy = true
  }
  int_enum    = 1
  string_enum = "two"
}
resource "enum_mod_nested_res" "sink3" {
  lifecycle {
    create_before_destroy = true
  }
  int_enum    = 1
  string_enum = "two"
}
