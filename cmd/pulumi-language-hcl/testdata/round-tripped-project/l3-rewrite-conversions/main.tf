terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "direct" {
  boolean      = "true"
  float        = "3.14"
  integer      = "42"
  string       = false
  number_array = ["-1", "0", "1"]
  boolean_map = {
    "t" = "true"
    "f" = "false"
  }
}
module "converted" {
  source      = "./converted"
  boolean     = "false"
  float       = "2.5"
  integer     = "7"
  string      = true
  numberArray = ["10", "11"]
  booleanMap = {
    "left"  = "true"
    "right" = "false"
  }
}
