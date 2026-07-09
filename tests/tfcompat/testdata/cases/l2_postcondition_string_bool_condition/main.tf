resource "simple_resource" "guarded" {
  input_one = "a"
  input_two = false

  lifecycle {
    postcondition {
      condition     = self.result == "a-false" ? "true" : "false"
      error_message = "result must be a-false"
    }
  }
}
