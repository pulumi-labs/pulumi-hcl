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

resource "simple_resource" "a" {
  provider  = simple.by_key["a"]
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.a.prefix_result
}
