variable "password" {
  type      = string
  sensitive = true
  default   = "hunter2"
}

module "db" {
  source   = "./modules/db"
  password = var.password
}

# The sensitive mark must also flow back OUT: a value the module derived from
# the sensitive input is read here and must still be marked sensitive.
output "outer_is_sensitive" {
  value = issensitive(module.db.connection)
}

# The actual value must survive the round trip unchanged. The drivers compare
# output values (not marks), so a root output carrying a sensitive value must be
# declared sensitive to satisfy OpenTofu; the value itself is what's compared.
output "connection_value" {
  value     = module.db.connection
  sensitive = true
}
