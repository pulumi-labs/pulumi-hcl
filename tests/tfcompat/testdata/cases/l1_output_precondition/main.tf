variable "enabled" {
  type    = bool
  default = false
}

output "result" {
  value = "ok"

  precondition {
    condition     = var.enabled
    error_message = "OUTPUT_PRECONDITION_FAILED"
  }
}
