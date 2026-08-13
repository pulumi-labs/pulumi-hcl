# A count meta-argument evaluating to null.
#
# OpenTofu rejects this with a clean diagnostic:
#   "The given "count" argument value is null. An argument for "count" must not
#    be null."
#
# pulumi-hcl's EvaluateCount checks IsKnown (null is known) but never IsNull, so
# it converts the null to a null number and calls AsBigFloat() on it, which
# panics ("value is null") and crashes the language host.
variable "n" {
  type    = number
  default = null
}

resource "simple_resource" "r" {
  count     = var.n
  input_one = "x"
  input_two = false
}
