resource "nested_cluster" "this" {
  count = 1
}

output "issuer" {
  value = nested_cluster.this[0].identity[0].oidc[0].issuer
}
