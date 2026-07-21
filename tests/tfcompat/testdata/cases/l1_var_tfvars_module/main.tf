# `terraform.tfvars` files are loaded automatically for the root module only:
# the one under ./declares sets `who` and is never read.
module "declares" {
  source = "./declares"
}

output "declares" {
  value = module.declares.who
}
