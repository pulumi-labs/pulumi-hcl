terraform {
  required_providers {
    discriminated-union = {
      source  = "pulumi/discriminated-union"
      version = "31.0.0"
    }
  }
}

resource "discriminated-union_example" "example1" {
  union_of = {
    "discriminantKind" = "variant1"
    "field1"           = "v1 union"
  }
  array_of_union_of = [{
    "discriminantKind" = "variant1"
    "field1"           = "v1 array(union)"
  }]
}
resource "discriminated-union_example" "example2" {
  union_of = {
    "discriminantKind" = "variant2"
    "field2"           = "v2 union"
  }
  array_of_union_of = [{
    "discriminantKind" = "variant2"
    "field2"           = "v2 array(union)"
    }, {
    "discriminantKind" = "variant1"
    "field1"           = "v1 array(union)"
  }]
}
