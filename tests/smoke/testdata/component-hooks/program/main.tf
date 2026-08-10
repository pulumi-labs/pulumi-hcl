# Never true, so the precondition on the component below must fail the update.
variable "enabled" {
  type    = bool
  default = false
}

resource "guardedmodule_module" "guarded" {
  name = "world"

  lifecycle {
    precondition {
      condition     = var.enabled
      error_message = "PRECONDITION_FAILED: the module must be enabled"
    }
  }
}

output "greeting" {
  value = guardedmodule_module.guarded.greeting
}
