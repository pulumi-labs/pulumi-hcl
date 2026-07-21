resource "simple_resource" "r" {
  input_one = "a"
  input_two = false

  lifecycle {
    ignore_changes = [input_one]
  }
}

output "input_one" {
  value = simple_resource.r.input_one
}
