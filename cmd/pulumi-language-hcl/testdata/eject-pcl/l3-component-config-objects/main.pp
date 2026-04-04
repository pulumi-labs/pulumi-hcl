config "plainNumberArray" "list(int)" {
}

config "plainBooleanMap" "map(bool)" {
}

config "secretNumberArray" "list(int)" {
}

config "secretBooleanMap" "map(bool)" {
}

component "plain" "./primitiveComponent" {
  numberArray = plainNumberArray
  booleanMap  = plainBooleanMap
}

component "secret" "./primitiveComponent" {
  numberArray = secretNumberArray
  booleanMap  = secretBooleanMap
}

