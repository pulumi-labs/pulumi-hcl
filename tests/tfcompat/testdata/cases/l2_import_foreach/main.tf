resource "importable_resource" "foo" {
  for_each = toset(["a", "b"])
}

# OpenTofu 1.7+ supports for_each on the import block itself, expanding one
# block into an import per element with each.key/each.value in scope.
import {
  for_each = { a = "id-a", b = "id-b" }
  to       = importable_resource.foo[each.key]
  id       = each.value
}

output "names" {
  value = { for k, r in importable_resource.foo : k => r.name }
}

# An import address may also use a dynamic index key without for_each.
variable "which" {
  default = "b"
}

resource "importable_resource" "dyn" {
  for_each = toset(["a", "b"])
}

import {
  to = importable_resource.dyn[var.which]
  id = "id-dyn"
}

output "dyn_names" {
  value = { for k, r in importable_resource.dyn : k => r.name }
}
