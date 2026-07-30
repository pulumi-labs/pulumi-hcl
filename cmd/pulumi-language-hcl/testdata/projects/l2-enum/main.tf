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
resource "enum_deluxe" "sink4" {
  lifecycle {
    create_before_destroy = true
  }
  number_enum   = 0.1
  wordy_enum    = "It's got apostrophes"
  array_of_enum = ["one", "two"]
  map_of_enum = {
    "small" = 1
    "large" = 2
  }
  holder = {
    size  = 2
    color = "one"
  }
  union_enum = "A Value With Spaces."
}
