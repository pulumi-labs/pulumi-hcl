variable "m" {
  type    = map(string)
  default = {}
}

# Mirrors the common community-module idiom:
#   cidr_block = lookup(ingress.value, "cidr_block", null)
# A null `default` lets the caller pass "leave this attribute unset". Wrapping
# the calls in an object output keeps `null` representable in state (tofu
# omits top-level null outputs from terraform.tfstate, which would mask the
# value rather than test it).
output "results" {
  value = {
    missing_with_null   = lookup(var.m, "missing", null)
    present_with_null   = lookup({ k = "v" }, "k", null)
    missing_with_string = lookup(var.m, "missing", "fallback")
  }
}
