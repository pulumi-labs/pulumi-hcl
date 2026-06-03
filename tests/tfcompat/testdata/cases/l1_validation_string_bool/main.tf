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

output "name" {
  value = var.name
}
