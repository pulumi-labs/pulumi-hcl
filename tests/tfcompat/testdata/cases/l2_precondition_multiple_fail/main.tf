variable "x" {
  type    = number
  default = 1
}

resource "simple_resource" "guarded" {
  input_one = "hi"

  lifecycle {
    precondition {
      condition     = var.x > 5
      error_message = "PRECOND_ONE_FAILS"
    }
    precondition {
      condition     = var.x > 10
      error_message = "PRECOND_TWO_FAILS"
    }
  }
}
