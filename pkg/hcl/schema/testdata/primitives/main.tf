variable "name" {
  type        = string
  description = "The resource name."
  nullable    = false
}

variable "count" {
  type    = number
  default = 1
}

variable "enabled" {
  type    = bool
  default = true
}

output "id" {
  value       = "static"
  description = "The generated identifier."
}

output "label" {
  value = "${var.name}-${var.count}"
}
