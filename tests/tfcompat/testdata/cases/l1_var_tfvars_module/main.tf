# `terraform.tfvars` files are loaded automatically for the root module only.
# The ones under ./declares and ./silent are never read.
module "declares" {
  source = "./declares"
}

module "silent" {
  source = "./silent"
}

output "declares" {
  value = module.declares.who
}

output "silent" {
  value = module.silent.who
}
