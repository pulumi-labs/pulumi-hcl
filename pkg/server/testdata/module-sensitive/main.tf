variable "password" {
  type = string
}

# issensitive surfaces the inbound mark as a plain bool, so whether the secret
# arrived sensitive is observable in an output without leaking the value.
output "password_is_sensitive" {
  value = issensitive(var.password)
}

# A value derived from the secret input; its sensitivity propagates by
# derivation, so the mark must travel back out across the component boundary.
output "connection" {
  value = "conn:${var.password}"
}
