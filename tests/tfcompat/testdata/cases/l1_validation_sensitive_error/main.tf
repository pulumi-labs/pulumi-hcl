variable "password" {
  type      = string
  sensitive = true
  default   = "abc"

  validation {
    condition     = length(var.password) >= 8
    error_message = "Password is too short: '${var.password}'"
  }
}

output "ok" {
  value = "ok"
}
