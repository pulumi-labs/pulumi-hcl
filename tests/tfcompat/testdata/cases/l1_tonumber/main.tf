# Terraform's `tonumber` parses the whole string as a decimal number, so
# fractional, exponential, and negative representations round-trip exactly. A
# `%d`-style parse that stops at the first non-digit would truncate "3.14" to 3,
# "1e2" to 1, and "-2.5" to -2.
output "pi" {
  value = tonumber("3.14")
}

output "exp" {
  value = tonumber("1e2")
}

output "neg" {
  value = tonumber("-2.5")
}

output "int" {
  value = tonumber("42")
}
