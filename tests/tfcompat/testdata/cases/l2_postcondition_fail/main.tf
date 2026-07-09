resource "simple_resource" "guarded" {
  input_one = "a"
  input_two = false

  lifecycle {
    postcondition {
      condition     = self.result == "expected-different-value"
      error_message = "POSTCONDITION_VIOLATED"
    }
  }
}
