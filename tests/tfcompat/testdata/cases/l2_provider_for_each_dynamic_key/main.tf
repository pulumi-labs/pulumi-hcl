variable "prefixes" {
  type = map(string)
  default = {
    a = "alpha"
    b = "beta"
  }
}

provider "simple" {
  alias    = "by_key"
  for_each = var.prefixes
  prefix   = each.value
}

resource "simple_resource" "r" {
  for_each  = var.prefixes
  provider  = simple.by_key[each.key]
  input_one = each.key
  input_two = true
}

output "prefix_results" {
  value = { for k, r in simple_resource.r : k => r.prefix_result }
}
