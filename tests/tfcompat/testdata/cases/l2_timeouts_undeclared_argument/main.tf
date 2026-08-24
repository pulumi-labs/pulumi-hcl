resource "timeoutable_resource" "test" {
  input_one = "hello"

  timeouts {
    read = "1m"
  }
}
