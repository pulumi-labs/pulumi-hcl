# A provider-free module declaring a single variable, used by
# TestModuleConstructRejectsUnknownInput to exercise input validation without
# resolving providers or running the engine.
variable "name" {
  type = string
}

output "greeting" {
  value = "hello ${var.name}"
}
