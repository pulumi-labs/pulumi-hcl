# OpenTofu's `sensitive`, `nonsensitive`, and `issensitive` all declare their
# value parameter with AllowNull, so a null argument is accepted (it marks /
# unmarks / inspects the null value) rather than erroring. The null is wrapped
# in an object so the output value itself is non-null and survives in state on
# both runtimes, isolating the argument-acceptance behavior under test.
output "sensitive_null" {
  value     = { v = sensitive(null) }
  sensitive = true
}

output "nonsensitive_null" {
  value     = { v = nonsensitive(sensitive(null)) }
  sensitive = true
}

output "issensitive_null" {
  value = issensitive(null)
}

# A non-null sensitive value round-trips its value through state unchanged.
output "sensitive_value" {
  value     = sensitive("secret-text")
  sensitive = true
}
