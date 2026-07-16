terraform {
  required_version = ">= 1.0"
}

# terraform_data.input/output are dynamically typed. A set fed into input must
# come back out of output still typed as a set, so an == comparison against a
# set of the same members is true.
resource "terraform_data" "set_input" {
  input = toset(["c", "a", "b", "a"])
}

# The raw serialized value is identical on both paths (a sorted array)...
output "set_output" {
  value = terraform_data.set_input.output
}

# ...but only OpenTofu preserves the set *type*, so this equality holds there
# and pulumi-hcl (which returns the output as a tuple) reports false.
output "set_is_set" {
  value = terraform_data.set_input.output == toset(["a", "b", "c"])
}
