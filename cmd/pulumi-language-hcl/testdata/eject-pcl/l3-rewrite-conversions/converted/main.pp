resource "res" "primitive:index:Resource" {
  boolean     = boolean
  float       = float
  integer     = integer
  string      = string
  numberArray = numberArray
  booleanMap  = booleanMap
}

config "boolean" "bool" {
}

config "float" "number" {
}

config "integer" "number" {
}

config "string" "string" {
}

config "numberArray" "list(int)" {
}

config "booleanMap" "map(bool)" {
}

