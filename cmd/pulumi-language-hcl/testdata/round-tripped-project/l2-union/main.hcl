terraform {
  required_providers {
    union = {
      source  = "pulumi/union"
      version = "18.0.0"
    }
  }
}

resource "union_example" "stringOrIntegerExample1" {
  string_or_integer_property = 42
}
resource "union_example" "stringOrIntegerExample2" {
  string_or_integer_property = "forty two"
}
resource "union_example" "mapMapUnionExample" {
  map_map_union_property = {
    "key1" = {
      "key1a" = "value1a"
    }
  }
}
resource "union_example" "stringEnumUnionListExample" {
  string_enum_union_list_property = ["Listen", "Send", "NotAnEnumValue"]
}
resource "union_example" "safeEnumExample" {
  typed_enum_property = "Block"
}
resource "union_enumoutput" "enumOutputExample" {
  name = "example"
}
resource "union_example" "outputEnumExample" {
  typed_enum_property = union_enumoutput.enumOutputExample.type
}
output "mapMapUnionOutput" {
  value = union_example.mapMapUnionExample.map_map_union_property
}
