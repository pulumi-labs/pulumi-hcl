resource "replacer_resource" "guarded" {
  force = "a"

  lifecycle {
    prevent_destroy = true
  }
}

output "result" {
  value = replacer_resource.guarded.result
}
