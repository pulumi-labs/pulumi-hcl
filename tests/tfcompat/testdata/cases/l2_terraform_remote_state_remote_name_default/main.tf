variable "org" { type = string }
variable "hostname" { type = string }
variable "token" { type = string }
variable "name" { type = string }

data "terraform_remote_state" "rs" {
  backend   = "remote"
  workspace = "default"
  config = {
    organization = var.org
    hostname     = var.hostname
    token        = var.token
    workspaces   = { name = var.name }
  }
}

output "greeting" { value = data.terraform_remote_state.rs.outputs.greeting }
output "number"   { value = data.terraform_remote_state.rs.outputs.number }
