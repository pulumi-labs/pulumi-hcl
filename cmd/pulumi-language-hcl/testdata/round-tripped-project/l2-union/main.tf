terraform {
  required_providers {
    union = {
      source  = "pulumi/union"
      version = "18.0.0"
    }
  }
}

resource "union_example" "stringOrIntegerExample1" {
  lifecycle {
    create_before_destroy = true
  }
  string_or_integer_property = 42
}
resource "union_example" "stringOrIntegerExample2" {
  lifecycle {
    create_before_destroy = true
  }
  string_or_integer_property = "forty two"
}
resource "union_example" "mapMapUnionExample" {
  lifecycle {
    create_before_destroy = true
  }
  map_map_union_property = {
    "key1" = {
      "key1a" = "value1a"
    }
  }
}
// List<Union<String, Enum>> pattern
resource "union_example" "stringEnumUnionListExample" {
  lifecycle {
    create_before_destroy = true
  }
  string_enum_union_list_property = ["Listen", "Send", "NotAnEnumValue"]
}
// Safe enum: literal string matching an enum value
resource "union_example" "safeEnumExample" {
  lifecycle {
    create_before_destroy = true
  }
  typed_enum_property = "Block"
}
// Output enum: output from another resource used as enum input
resource "union_enum_output" "enumOutputExample" {
  lifecycle {
    create_before_destroy = true
  }
  name = "example"
}
resource "union_example" "outputEnumExample" {
  lifecycle {
    create_before_destroy = true
  }
  typed_enum_property = union_enum_output.enumOutputExample.type
}
output "mapMapUnionOutput" {
  value = union_example.mapMapUnionExample.map_map_union_property
}
