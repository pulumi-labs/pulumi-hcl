variable "trigger_failure" {
  type    = bool
  default = true
}

resource "simple_resource" "guarded" {
  input_one = "value"

  lifecycle {
    precondition {
      condition     = !var.trigger_failure
      error_message = "PRECONDITION_VIOLATED"
    }
  }
}
