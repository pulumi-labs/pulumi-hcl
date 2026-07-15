# `count` and `for_each` are mutually exclusive on a single resource block.
# OpenTofu rejects the combination at plan time; pulumi-hcl must reject it too
# rather than silently letting one meta-argument win.
resource "simple_resource" "web" {
  count     = 2
  for_each  = toset(["a", "b"])
  input_one = "x"
  input_two = false
}
