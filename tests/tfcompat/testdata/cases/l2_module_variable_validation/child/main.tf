variable "name" {
  type = string

  validation {
    condition     = length(var.name) > 3
    error_message = "VALIDATION_FAILED_MODULE_VAR_TOO_SHORT"
  }
}

output "name" {
  value = var.name
}
