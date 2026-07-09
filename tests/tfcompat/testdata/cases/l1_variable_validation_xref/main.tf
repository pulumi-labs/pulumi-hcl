variable "min" {
  type    = number
  default = 3
}

variable "name" {
  type    = string
  default = "xy"

  validation {
    condition     = length(var.name) >= var.min
    error_message = "XREF_VALIDATION_FAILED"
  }
}

output "name" {
  value = var.name
}
