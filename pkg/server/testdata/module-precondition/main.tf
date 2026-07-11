variable "expected" {
  type    = string
  default = "ok"
}

resource "terraform_data" "guarded" {
  input = var.expected

  lifecycle {
    precondition {
      condition     = var.expected == "ok"
      error_message = "PRECONDITION_IN_MODULE"
    }
  }
}
