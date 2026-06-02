# OpenTofu's matchkeys unifies the element types of `keys` and `searchset`
# before comparing them, so a numeric key list can be matched against a string
# search set: number and string unify to string, and "2" == "2". A faithful
# implementation therefore returns the value whose key matches.
output "type_unified" {
  value = matchkeys(["a", "b", "c"], [1, 2, 3], ["2"])
}

# Same element types: the common case must keep working unchanged.
output "same_type" {
  value = matchkeys(["a", "b", "c"], ["x", "y", "x"], ["x"])
}
