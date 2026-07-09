resource "simple_resource" "example" {
  input_one = "hello"
}

check "result_check" {
  assert {
    condition     = simple_resource.example.result == "hello-false"
    error_message = "result did not match expected value"
  }
}
