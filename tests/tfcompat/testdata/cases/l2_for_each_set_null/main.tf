# A for_each over a set containing a null element.
#
# OpenTofu rejects this with a clean diagnostic:
#   "The given "for_each" argument value is unsuitable: "for_each" set includes
#    a null value, which is not allowed."
#
# pulumi-hcl's EvaluateForEach passes the set through its element-type and
# wholly-known guards (null is a known string), then calls AsString() on the
# null element, which panics ("value is null") and crashes the language host.
resource "simple_resource" "r" {
  for_each  = toset(["a", null])
  input_one = each.key
  input_two = false
}
