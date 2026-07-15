resource "importable_resource" "foo" {
  for_each = toset(["a", "b"])
}

import {
  to = importable_resource.foo["a"]
  id = "id-a"
}

import {
  to = importable_resource.foo["b"]
  id = "id-b"
}

output "names" {
  value = { for k, r in importable_resource.foo : k => r.name }
}
