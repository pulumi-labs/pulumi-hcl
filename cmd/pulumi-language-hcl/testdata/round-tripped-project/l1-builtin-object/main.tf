variable "aMap" {
  type = map(string)
}
output "entriesOutput" {
  value = entries(var.aMap)
}
output "lookupOutput" {
  value = lookup(var.aMap, "keyPresent", "default")
}
output "lookupOutputDefault" {
  value = lookup(var.aMap, "keyMissing", "default")
}
