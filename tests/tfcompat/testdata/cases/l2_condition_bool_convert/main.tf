variable "name" {
  type    = string
  default = "widget"

  # OpenTofu converts the condition result to bool, so a string that is
  # convertible to bool ("true"/"false") is a valid condition value.
  validation {
    condition     = length(var.name) > 2 ? "true" : "false"
    error_message = "name must be longer than two characters"
  }
}

resource "terraform_data" "r" {
  input = var.name

  lifecycle {
    # Precondition results are likewise bool-converted by OpenTofu.
    precondition {
      condition     = var.name != "" ? "true" : "false"
      error_message = "name must not be empty"
    }

    postcondition {
      condition     = self.output == var.name ? "true" : "false"
      error_message = "output must echo the input"
    }
  }
}

output "name" {
  value = terraform_data.r.output
}
