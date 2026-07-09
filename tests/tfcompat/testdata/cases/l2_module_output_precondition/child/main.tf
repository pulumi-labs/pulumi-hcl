variable "ok" {
  type    = bool
  default = false
}

output "result" {
  value = "ok"

  precondition {
    condition     = var.ok
    error_message = "MODULE_OUTPUT_PRECONDITION_FAILED"
  }
}
