component "first" "./first" {
  input = second.untainted
}

component "second" "./second" {
  input = first.untainted
}

component "another" "./first" {
  input = join("", [for _, v in many : v.untainted ? "a" : "b"]) == "xyz"
}

component "many" "./second" {
  input = another.untainted
  options {
    range = 2
  }
}

