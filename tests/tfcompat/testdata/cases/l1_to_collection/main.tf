# OpenTofu's tolist / toset / tomap accept a tuple or object whose elements have
# differing types and unify them to a single common element type before building
# the collection. Here each input mixes numbers, strings, and bools, all of which
# unify to string, so the result is a homogeneous string collection.
output "to_list_mixed" {
  value = tolist([1, "a", true])
}

output "to_set_mixed" {
  value = toset([3, "1", 2])
}

output "to_map_mixed" {
  value = tomap({ a = 1, b = "x", c = true })
}
