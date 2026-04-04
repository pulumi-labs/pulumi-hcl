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

component "plain" "./primitiveComponent" {
  boolean = plainBool
  float   = plainNumber
  integer = plainInteger
  string  = plainString
}

component "secret" "./primitiveComponent" {
  boolean = secretBool
  float   = secretNumber
  integer = secretInteger
  string  = secretString
}

