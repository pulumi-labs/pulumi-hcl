variable "expected" {
  type    = string
  default = "ok"
}

resource "simple_resource" "guarded" {
  input_one = "ok"

  lifecycle {
    precondition {
      condition     = var.expected != "" ? "true" : "false"
      error_message = "expected must not be empty"
    }
  }
}
