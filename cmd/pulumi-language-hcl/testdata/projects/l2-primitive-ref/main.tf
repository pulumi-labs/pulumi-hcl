terraform {
  required_providers {
    primitive-ref = {
      source  = "pulumi/primitive-ref"
      version = "11.0.0"
    }
  }
}

resource "primitive-ref_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  data = {
    boolean    = false
    float      = 2.17
    integer    = -12
    string     = "Goodbye"
    bool_array = [false, true]
    string_map = {
      "two"   = "turtle doves"
      "three" = "french hens"
    }
  }
}
