resource "simple_resource" "first" {
  input_one = "first"
  input_two = true
}

resource "simple_resource" "second" {
  input_one = "second"
  input_two = false
  depends_on = [simple_resource.first]
}

output "first_result" {
  value = simple_resource.first.result
}

output "second_result" {
  value = simple_resource.second.result
}
