provider "simple" {
  alias  = "alt"
  prefix = "base"
}

resource "simple_resource" "r" {
  provider  = simple.alt
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
