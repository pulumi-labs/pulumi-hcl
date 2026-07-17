# `prevent_destroy` is set from a variable rather than a literal bool. OpenTofu
# accepts a non-literal `prevent_destroy` expression (a variable or a
# conditional) and evaluates it -- `prevent_destroy = var.flag` with flag=true
# protects the resource just like `prevent_destroy = true`.
#
# pulumi-hcl decodes the `prevent_destroy` expression with a nil HCL eval
# context, so any reference to a variable fails to resolve and the program is
# rejected with "Variables not allowed" before anything is created.
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
