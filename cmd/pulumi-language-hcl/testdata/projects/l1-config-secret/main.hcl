variable "aNumber" {
  type = number
}
output "roundtrip" {
  value = var.aNumber
}
output "theSecretNumber" {
  value = var.aNumber + 1.25
}
