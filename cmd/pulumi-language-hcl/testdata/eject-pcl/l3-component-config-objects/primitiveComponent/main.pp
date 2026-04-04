resource "res" "primitive:index:Resource" {
  boolean     = true
  float       = 3.5
  integer     = 3
  string      = "plain"
  numberArray = numberArray
  booleanMap  = booleanMap
}

config "numberArray" "list(int)" {
}

config "booleanMap" "map(bool)" {
}

