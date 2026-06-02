variable "org" { type = string }
variable "hostname" { type = string }
variable "token" { type = string }
variable "workspace" { type = string }

data "terraform_remote_state" "rs" {
  backend = "remote"
  config = {
    organization = var.org
    hostname     = var.hostname
    token        = var.token
    workspaces   = { name = var.workspace }
  }
}

output "greeting" {
  value = data.terraform_remote_state.rs.outputs.greeting
}

output "number" {
  value = data.terraform_remote_state.rs.outputs.number
}
