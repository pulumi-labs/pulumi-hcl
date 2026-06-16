variable "region" {
  type = string
}

locals {
  prefix = "${var.region}-app"
  port   = 8080
}

resource "random_pet" "name" {
  length = 2
}

output "prefix" {
  value = local.prefix
}

output "port" {
  value = local.port
}

# random_pet is a resource reference, which requires a provider schema to type.
# Schema generation does not resolve provider schemas, so this falls back to any.
output "pet_id" {
  value = random_pet.name.id
}
