config "plainBool" "bool" {
}

config "plainNumber" "number" {
}

config "plainInteger" "number" {
}

config "plainString" "string" {
}

config "plainNumericString" "string" {
}

config "secretNumber" "number" {
}

config "secretInteger" "number" {
}

config "secretString" "string" {
}

config "secretNumericString" "string" {
}

component "plainValues" "./conversionComponent" {
  boolean = plainString
  float   = plainInteger
  integer = plainNumericString
  string  = plainNumber
}

component "secretValues" "./conversionComponent" {
  boolean = secretString
  float   = secretInteger
  integer = secretNumericString
  string  = secretNumber
}

