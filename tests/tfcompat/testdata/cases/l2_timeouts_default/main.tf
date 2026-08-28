resource "timeoutable_resource" "test" {
  input_one = "hello"

  timeouts {
    default = "7m"
  }
}

output "timeouts" {
  value = timeoutable_resource.test.timeouts
}
