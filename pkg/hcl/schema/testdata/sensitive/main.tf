variable "secret_key" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "freeform" {
  description = "No type constraint, so it defaults to the object (any) type."
}

output "token" {
  value     = "x"
  sensitive = true
}
