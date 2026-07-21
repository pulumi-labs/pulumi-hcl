resource "importable_resource" "foo" {
  count = 2
}

# An import block's for_each accepts a map, a set of strings, or a tuple. For a
# tuple each.key is the element's index, so it can key a counted resource
# directly.
import {
  for_each = ["id-a", "id-b"]
  to       = importable_resource.foo[each.key]
  id       = each.value
}

output "names" {
  value = importable_resource.foo[*].name
}
