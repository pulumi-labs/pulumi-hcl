# `force` is ForceNew: this change plans a replacement, whose destroy half is
# refused by prevent_destroy at plan time.
resource "replacer_resource" "guarded" {
  force = "b"

  lifecycle {
    prevent_destroy = true
  }
}

output "result" {
  value = replacer_resource.guarded.result
}
