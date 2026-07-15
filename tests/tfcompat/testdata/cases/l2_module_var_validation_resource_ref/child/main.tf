resource "simple_resource" "r" {
  input_one = "a"
}

variable "gate" {
  type    = string
  default = "a"

  validation {
    condition     = simple_resource.r.result == "${var.gate}-false"
    error_message = "MODULE_GATE_VALIDATION_FAILED"
  }
}

output "gate" {
  value = var.gate
}

output "result" {
  value = simple_resource.r.result
}
