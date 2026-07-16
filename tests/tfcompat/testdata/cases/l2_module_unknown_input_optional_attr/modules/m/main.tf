# Declared type adds an optional attribute `c` that the source object lacks.
variable "cfg" {
  type = object({
    a = string
    c = optional(string, "def")
  })
}
# `.c` is valid only if the value carries the declared object type (with `c`).
output "c" { value = var.cfg.c }
