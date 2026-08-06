variable "sensitive_string" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "untyped_value" {
  description = "No type constraint, so it defaults to the any type."
}

output "sensitive_output" {
  value     = "x"
  sensitive = true
}
