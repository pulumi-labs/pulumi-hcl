variable "name" {
  type    = string
  default = "x"

  validation {
    condition     = length(var.name) > 3
    error_message = "VALIDATION_FAILED_NAME_TOO_SHORT"
  }
}

output "name" {
  value = var.name
}
