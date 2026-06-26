# We use `secret` to lift plain values to output space, then check we can index into them
variable "anObject" {
  type = object({property=string})
}
variable "anyObject" {
  type = any
}
locals {
  l = sensitive([1])
}
locals {
  m = sensitive({
    "key" = true
  })
}
locals {
  c = sensitive(var.anObject)
}
locals {
  o = sensitive({
    "property" = "value"
  })
}
locals {
  a = sensitive(var.anyObject)
}
output "l" {
  value = local.l[0]
}
output "m" {
  value = local.m["key"]
}
output "c" {
  value = local.c.property
}
output "o" {
  value = local.o.property
}
output "a" {
  value = local.a.property
}
