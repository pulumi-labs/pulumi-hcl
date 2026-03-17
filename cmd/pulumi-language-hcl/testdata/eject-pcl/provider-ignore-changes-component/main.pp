resource "withIgnoreChanges" "conformance-component:index:Simple" {
  value = true
  options {
    ignoreChanges = [value]
  }
}

resource "withoutIgnoreChanges" "conformance-component:index:Simple" {
  value = true
}

resource "simpleResource" "simple:index:Resource" {
  value = false
}

