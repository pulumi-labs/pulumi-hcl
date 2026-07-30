# Same replacement attempt as stage 1, driven through apply instead of plan.
resource "replacer_resource" "guarded" {
  force = "b"

  lifecycle {
    prevent_destroy = true
  }
}

output "result" {
  value = replacer_resource.guarded.result
}
