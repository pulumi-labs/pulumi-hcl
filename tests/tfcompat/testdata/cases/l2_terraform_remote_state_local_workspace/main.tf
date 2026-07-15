# OpenTofu's local backend supports named workspaces: with a non-default
# `workspace`, terraform_remote_state reads `<workspace_dir>/<workspace>/
# terraform.tfstate` instead of the default `path`. This fixture reads the
# "staging" workspace's state from states/staging/terraform.tfstate. Both
# runtimes must surface identical outputs.
data "terraform_remote_state" "rs" {
  backend   = "local"
  workspace = "staging"
  config = {
    workspace_dir = "states"
  }
}

output "who" {
  value = data.terraform_remote_state.rs.outputs.who
}

output "num" {
  value = data.terraform_remote_state.rs.outputs.num
}
