# Variable values supplied as strings (`-var` / TF_VAR_ / Pulumi config) must be
# parsed exactly as OpenTofu parses them: the declared type selects the parsing
# mode. Primitive types take the value literally (then convert it); complex types
# parse it as an HCL expression; an untyped variable keeps the literal string.
#
# `str`, `items`, `untyped`, and `anyTyped` are all given the identical value
# `["a", "b"]`, so the only thing that can produce their differing results is
# type-driven parsing. In particular `untyped` (no type) and `anyTyped`
# (`type = any`) diverge on that same input: a missing type parses literally,
# while `any` is a declared type and so parses as HCL.

variable "str" {
  type = string
}

variable "num" {
  type = number
}

variable "flag" {
  type = bool
}

variable "tags" {
  type = map(string)
}

variable "items" {
  type = list(string)
}

variable "untyped" {}

variable "anyTyped" {
  type = any
}

# Primitive: kept literal, so a JSON/HCL-looking string survives verbatim
# (interior whitespace included) rather than being parsed into a structure.
output "str" {
  value = var.str
}

# Primitive: literal then converted to the declared type.
output "num" {
  value = var.num
}

output "flag" {
  value = var.flag
}

# Complex: parsed as an HCL expression, so the idiomatic `{ key = "value" }`
# form is accepted (it is not valid JSON).
output "tags" {
  value = var.tags
}

# Complex: the same `["a", "b"]` that stays a literal string for `str` parses
# into a list here.
output "items" {
  value = var.items
}

# Untyped: kept literal, like a primitive — not HCL- or JSON-parsed.
output "untyped" {
  value = var.untyped
}

# Explicit `any`: a declared type, so the same `["a", "b"]` that stays a literal
# string for `untyped` parses into a list here.
output "anyTyped" {
  value = var.anyTyped
}
