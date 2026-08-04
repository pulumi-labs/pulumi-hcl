resource "simple_resource" "a" {
  input_one = "alpha"
  input_two = true
}

resource "simple_resource" "b" {
  input_one = "beta"
}

output "result_a" {
  value = simple_resource.a.result
}

output "result_b" {
  value = simple_resource.b.result
}
