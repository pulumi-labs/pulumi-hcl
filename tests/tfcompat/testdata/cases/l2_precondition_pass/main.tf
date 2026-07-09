variable "expected" {
  type    = string
  default = "ok"
}

resource "simple_resource" "guarded" {
  input_one = "ok"

  lifecycle {
    precondition {
      condition     = var.expected == "ok"
      error_message = "should never fire"
    }
  }
}
