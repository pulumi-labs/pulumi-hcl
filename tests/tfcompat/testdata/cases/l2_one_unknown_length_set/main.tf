resource "simple_resource" "upstream" {
  input_one = "a"
  input_two = true
}

output "the_one" {
  value = one(toset([simple_resource.upstream.result, "fixed"]))
}
