config "aNumber" "number" {
}

output "roundtrip" {
  value = aNumber
}

output "theSecretNumber" {
  value = aNumber + 1.25
}

