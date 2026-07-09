resource "nested_cluster" "this" {
}

output "issuer" {
  value = nested_cluster.this.identity[0].oidc[0].issuer
}
