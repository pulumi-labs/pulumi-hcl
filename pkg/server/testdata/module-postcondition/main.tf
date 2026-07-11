variable "expected" {
  type    = string
  default = "ok"
}

resource "terraform_data" "guarded" {
  input = var.expected

  lifecycle {
    postcondition {
      condition     = self.input == "ok"
      error_message = "POSTCONDITION_IN_MODULE"
    }
  }
}
