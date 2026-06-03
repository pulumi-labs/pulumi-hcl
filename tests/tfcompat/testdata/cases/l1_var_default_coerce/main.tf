# A variable's `default` is coerced to its declared `type` exactly like a
# supplied value. OpenTofu converts each element/attribute to satisfy the type
# constraint; the raw literal types in the default must not survive.

variable "lst" {
  type    = list(string)
  default = ["a", 1, true]
}

variable "ports" {
  type    = set(number)
  default = ["8080", "80", "8080"]
}

variable "m" {
  type    = map(string)
  default = { a = 1, b = true }
}

output "lst"   { value = var.lst }
output "ports" { value = var.ports }
output "m"     { value = var.m }
