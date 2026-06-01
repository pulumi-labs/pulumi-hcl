variable "aString" {
  type = string
}
output "lengthResult" {
  value = length(var.aString)
}
output "splitResult" {
  value = split("-", var.aString)
}
output "joinResult" {
  value = join("|", split("-", var.aString))
}
output "interpolateResult" {
  value ="prefix-${var.aString}"
}
