variable "string_value" {
  type        = string
  description = "The resource name."
  nullable    = false
}

variable "number_value" {
  type    = number
  default = 1
}

variable "bool_value" {
  type    = bool
  default = true
}

output "string_output" {
  value       = "static"
  description = "The generated identifier."
}

output "template_output" {
  value = "${var.string_value}-${var.number_value}"
}
