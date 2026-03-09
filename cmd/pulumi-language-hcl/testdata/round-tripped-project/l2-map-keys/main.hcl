terraform {
  required_providers {
    plain = {
      source  = "pulumi/plain"
      version = "13.0.0"
    }
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
    primitive-ref = {
      source  = "pulumi/primitive-ref"
      version = "11.0.0"
    }
    ref-ref = {
      source  = "pulumi/ref-ref"
      version = "12.0.0"
    }
  }
}

resource "primitive_resource" "prim" {
  boolean      = false
  float        = 2.17
  integer      = -12
  string       = "Goodbye"
  number_array = [0, 1]
  boolean_map = {
    "my key" = false
    "my.key" = true
    "my-key" = false
    "my_key" = true
    "MY_KEY" = false
    "myKey"  = true
  }
}
resource "primitive-ref_resource" "ref" {
  data = {
    boolean    = false
    float      = 2.17
    integer    = -12
    string     = "Goodbye"
    bool_array = [false, true]
    string_map = {
      "my key" = "one"
      "my.key" = "two"
      "my-key" = "three"
      "my_key" = "four"
      "MY_KEY" = "five"
      "myKey"  = "six"
    }
  }
}
resource "ref-ref_resource" "rref" {
  data = {
    inner_data = {
      boolean    = false
      float      = -2.17
      integer    = 123
      string     = "Goodbye"
      bool_array = []
      string_map = {
        "my key" = "one"
        "my.key" = "two"
        "my-key" = "three"
        "my_key" = "four"
        "MY_KEY" = "five"
        "myKey"  = "six"
      }
    }
    boolean    = true
    float      = 4.5
    integer    = 1024
    string     = "Hello"
    bool_array = []
    string_map = {
      "my key" = "one"
      "my.key" = "two"
      "my-key" = "three"
      "my_key" = "four"
      "MY_KEY" = "five"
      "myKey"  = "six"
    }
  }
}
resource "plain_resource" "plains" {
  data = {
    inner_data = {
      boolean    = false
      float      = 2.17
      integer    = -12
      string     = "Goodbye"
      bool_array = [false, true]
      string_map = {
        "my key" = "one"
        "my.key" = "two"
        "my-key" = "three"
        "my_key" = "four"
        "MY_KEY" = "five"
        "myKey"  = "six"
      }
    }
    boolean    = true
    float      = 4.5
    integer    = 1024
    string     = "Hello"
    bool_array = [true, false]
    string_map = {
      "my key" = "one"
      "my.key" = "two"
      "my-key" = "three"
      "my_key" = "four"
      "MY_KEY" = "five"
      "myKey"  = "six"
    }
  }
  non_plain_data = {
    inner_data = {
      boolean    = false
      float      = 2.17
      integer    = -12
      string     = "Goodbye"
      bool_array = [false, true]
      string_map = {
        "my key" = "one"
        "my.key" = "two"
        "my-key" = "three"
        "my_key" = "four"
        "MY_KEY" = "five"
        "myKey"  = "six"
      }
    }
    boolean    = true
    float      = 4.5
    integer    = 1024
    string     = "Hello"
    bool_array = [true, false]
    string_map = {
      "my key" = "one"
      "my.key" = "two"
      "my-key" = "three"
      "my_key" = "four"
      "MY_KEY" = "five"
      "myKey"  = "six"
    }
  }
}
