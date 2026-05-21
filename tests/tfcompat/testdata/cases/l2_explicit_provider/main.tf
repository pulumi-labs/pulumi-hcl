provider "simple" {
  alias  = "prefixed"
  prefix = "hello"
}

resource "simple_resource" "a" {
  provider  = simple.prefixed
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.a.prefix_result
}
