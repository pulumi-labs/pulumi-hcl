resource "timeoutable_resource" "test" {
  input_one = "hello"
}

output "is_null" {
  value = timeoutable_resource.test.timeouts == null
}
