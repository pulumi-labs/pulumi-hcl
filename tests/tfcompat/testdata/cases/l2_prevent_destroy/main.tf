resource "simple_resource" "protected" {
  input_one = "x"
  input_two = true

  lifecycle {
    prevent_destroy = true
  }
}

output "result" {
  value = simple_resource.protected.result
}
