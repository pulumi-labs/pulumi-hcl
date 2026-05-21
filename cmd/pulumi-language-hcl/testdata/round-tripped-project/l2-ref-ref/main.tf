terraform {
  required_providers {
    ref-ref = {
      source  = "pulumi/ref-ref"
      version = "12.0.0"
    }
  }
}

resource "ref-ref_resource" "res" {
  data = {
    inner_data = {
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
    boolean    = true
    float      = 4.5
    integer    = 1024
    string     = "Hello"
    bool_array = []
    string_map = {
      "x" = "100"
      "y" = "200"
    }
  }
}
