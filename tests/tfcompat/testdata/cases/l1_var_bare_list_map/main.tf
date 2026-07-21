# `type = list` and `type = map` are OpenTofu's pre-0.12 shorthands for
# `list(any)` and `map(any)`: the bare keyword is accepted and the value's
# elements are unified to a single common element type.

variable "l" {
  type    = list
  default = ["a", 1, true]
}

variable "m" {
  type    = map
  default = { x = 1, y = "two" }
}

output "l" { value = var.l }
output "m" { value = var.m }
