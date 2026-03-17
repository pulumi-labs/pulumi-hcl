resource "parent" "simple:index:Resource" {
  value = true
}

resource "res" "conformance-component:index:Simple" {
  value = true
  options {
    parent = parent
    aliases = [{
      noParent = true
    }]
  }
}

resource "simpleResource" "simple:index:Resource" {
  value = false
}

