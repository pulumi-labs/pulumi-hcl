resource "simple_resource" "r" {
  input_one = "base"
  input_two = false
}

output "input_one" { value = simple_resource.r.input_one }
output "input_two" { value = simple_resource.r.input_two }
