# `prevent_destroy` is set from a variable rather than a literal bool. The
# expression is evaluated at runtime, so `prevent_destroy = var.flag` with
# flag=true protects the resource just like `prevent_destroy = true`.
variable "flag" {
  type = bool
}

resource "simple_resource" "t" {
  input_one = "x"
  lifecycle {
    prevent_destroy = var.flag
  }
}

output "result" {
  value = simple_resource.t.result
}
