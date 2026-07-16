terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "x" {
  input = toset(["a", "b", "c"])

  lifecycle {
    postcondition {
      condition     = self.output == toset(["a", "b", "c"])
      error_message = "output is not a set equal to input"
    }
  }
}

output "ok" {
  value = terraform_data.x.output
}
