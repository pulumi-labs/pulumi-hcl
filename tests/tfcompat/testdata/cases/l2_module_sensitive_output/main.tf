module "secret" {
  source = "./modules/secret"
}

# A module output declared `sensitive = true` must carry the sensitive mark into
# the calling module. `issensitive` exposes the mark as a plain bool, so the
# behavior is observable in outputs without leaking the value (and without
# forcing this root output to itself be sensitive).
output "is_sensitive" {
  value = issensitive(module.secret.token)
}

# The mark must also propagate through an expression that derives from the
# sensitive module output.
output "derived_is_sensitive" {
  value = issensitive("tok-${module.secret.token}")
}
