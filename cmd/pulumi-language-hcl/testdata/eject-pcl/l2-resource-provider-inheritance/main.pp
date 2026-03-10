resource "provider" "pulumi:providers:simple" {
}

resource "parent1" "simple:index:Resource" {
  value = true
  options {
    provider = provider
  }
}

resource "child1" "simple:index:Resource" {
  value = true
  options {
    parent = parent1
  }
}

resource "parent2" "primitive:index:Resource" {
  boolean     = false
  float       = 0
  integer     = 0
  string      = ""
  numberArray = []
  booleanMap  = {}
}

resource "child2" "simple:index:Resource" {
  value = true
  options {
    parent = parent2
  }
}

resource "child3" "primitive:index:Resource" {
  boolean     = false
  float       = 0
  integer     = 0
  string      = ""
  numberArray = []
  booleanMap  = {}
  options {
    parent = parent1
  }
}

