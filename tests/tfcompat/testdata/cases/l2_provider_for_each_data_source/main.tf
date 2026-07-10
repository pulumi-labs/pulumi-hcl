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

data "simple_lookup" "d" {
  provider = simple.by_key["b"]
  query    = "hello"
}

output "prefix_result" {
  value = data.simple_lookup.d.prefix_result
}
