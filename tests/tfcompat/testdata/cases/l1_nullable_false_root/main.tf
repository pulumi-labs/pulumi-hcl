# A root variable declared `nullable = false` with a default. Assigning the
# null literal (here via stack config / -var) must substitute the variable's
# default, exactly as it does for a module input. The declared type is a
# complex (non-primitive) type, so the config string `null` parses as the HCL
# null literal rather than the four-character string "null".
variable "v" {
  type     = object({ a = string, b = optional(number, 5) })
  default  = { a = "aa" }
  nullable = false
}

# false in both runtimes if the default was substituted; true only if the
# variable was left null.
output "is_null" { value = var.v == null }

# The substituted, optional-filled default.
output "value" { value = var.v }
