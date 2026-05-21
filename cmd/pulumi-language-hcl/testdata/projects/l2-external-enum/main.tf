terraform {
  required_providers {
    enum = {
      source  = "pulumi/enum"
      version = "30.0.0"
    }
    extenumref = {
      source  = "pulumi/extenumref"
      version = "32.0.0"
    }
  }
}

resource "enum_res" "myRes" {
  int_enum    = 1
  string_enum = "one"
}
resource "extenumref_sink" "mySink" {
  string_enum = "two"
}
