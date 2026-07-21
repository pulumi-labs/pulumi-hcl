resource "pfx_matrix" "t" {
  matrix = {
    left  = [{ name = "a" }]
    right = [{ name = "c" }]
  }
}

output "matrix" {
  value = jsonencode(pfx_matrix.t.matrix)
}
