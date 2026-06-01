# The builtin `terraform` provider's terraform_remote_state data source has no
# installable plugin. OpenTofu reads it internally; pulumi-language-hcl lowers
# the `local` backend onto the pulumi-terraform package's getLocalReference
# invoke. Both read the same on-disk state fixture (remote.tfstate) and must
# expose identical outputs.
data "terraform_remote_state" "rs" {
  backend = "local"
  config = {
    path = "remote.tfstate"
  }
}

output "greeting" {
  value = data.terraform_remote_state.rs.outputs.greeting
}

output "number" {
  value = data.terraform_remote_state.rs.outputs.number
}

output "items" {
  value = data.terraform_remote_state.rs.outputs.items
}
