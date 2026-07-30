resource "simple_resource" "keep" {
  input_one = "hello"

  lifecycle {
    prevent_destroy = true
  }
}

output "result" {
  value = simple_resource.keep.result
}
