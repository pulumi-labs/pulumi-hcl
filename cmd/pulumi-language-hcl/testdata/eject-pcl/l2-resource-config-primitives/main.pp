resource "plain" "primitive:index:Resource" {
  boolean     = plainBool
  float       = plainNumber
  integer     = plainInteger
  string      = plainString
  numberArray = [-1, 0, 1]
  booleanMap = {
    "t" = true
    "f" = false
  }
}

resource "secret" "primitive:index:Resource" {
  boolean     = secretBool
  float       = secretNumber
  integer     = secretInteger
  string      = secretString
  numberArray = [-2, 0, 2]
  booleanMap = {
    "t" = true
    "f" = false
  }
}

config "plainBool" "bool" {
}

config "plainNumber" "number" {
}

config "plainInteger" "number" {
}

config "plainString" "string" {
}

config "secretBool" "bool" {
}

config "secretNumber" "number" {
}

config "secretInteger" "number" {
}

config "secretString" "string" {
}

