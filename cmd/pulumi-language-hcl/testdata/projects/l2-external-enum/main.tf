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
  lifecycle {
    create_before_destroy = true
  }
  int_enum    = 1
  string_enum = "one"
}
resource "extenumref_sink" "mySink" {
  lifecycle {
    create_before_destroy = true
  }
  string_enum = "two"
}
