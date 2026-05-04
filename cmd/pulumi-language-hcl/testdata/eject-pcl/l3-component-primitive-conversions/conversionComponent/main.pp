resource "res" "primitive:index:Resource" {
  boolean     = boolean
  float       = float
  integer     = integer
  string      = string
  numberArray = [2, 42, 6.5]
  booleanMap = {
    "fromBool"   = true
    "fromString" = true
  }
}

config "boolean" "bool" {
}

config "float" "number" {
}

config "integer" "number" {
}

config "string" "string" {
}

output "boolean" {
  value = res.boolean
}

output "float" {
  value = res.float
}

output "integer" {
  value = res.integer
}

output "string" {
  value = res.string
}

