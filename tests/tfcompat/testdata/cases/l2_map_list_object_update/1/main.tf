# One key's list grows, so the map's values no longer all have the same
# length. The attribute type is map(list(object)), so this is still a single
# well-typed value.
resource "pfx_matrix" "t" {
  matrix = {
    left  = [{ name = "a" }, { name = "b" }]
    right = [{ name = "c" }]
  }
}

output "matrix" {
  value = jsonencode(pfx_matrix.t.matrix)
}
