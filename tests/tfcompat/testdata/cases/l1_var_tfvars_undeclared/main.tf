# The root `terraform.tfvars` sets `who`, which only the child module declares,
# and `undeclared`. Neither is a root variable, so both values go nowhere and
# are reported. An undeclared name is never evaluated, so `undeclared`'s
# expression is not an error even though it could not be evaluated here.
module "child" {
  source = "./child"
}

output "who" {
  value = module.child.who
}
