resource "timeoutable_resource" "test" {
  input_one = "hello"

  timeouts {
    create = "5m"
  }
}

output "read_timeout" {
  value = timeoutable_resource.test.timeouts.read
}
