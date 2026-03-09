resource "stringOrIntegerExample1" "union:index:Example" {
  stringOrIntegerProperty = 42
}

resource "stringOrIntegerExample2" "union:index:Example" {
  stringOrIntegerProperty = "forty two"
}

resource "mapMapUnionExample" "union:index:Example" {
  mapMapUnionProperty = {
    "key1" = {
      "key1a" = "value1a"
    }
  }
}

resource "stringEnumUnionListExample" "union:index:Example" {
  stringEnumUnionListProperty = ["Listen", "Send", "NotAnEnumValue"]
}

resource "safeEnumExample" "union:index:Example" {
  typedEnumProperty = "Block"
}

resource "enumOutputExample" "union:index:EnumOutput" {
  name = "example"
}

resource "outputEnumExample" "union:index:Example" {
  typedEnumProperty = enumOutputExample.type
}

output "mapMapUnionOutput" {
  value = mapMapUnionExample.mapMapUnionProperty
}

