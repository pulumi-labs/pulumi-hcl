# terraform_remote_state exposes more than `outputs`: the data source stores its
# own `backend`, `config`, `workspace` and `defaults` arguments back as readable
# attributes, so a program can reference them (a module that takes the backend
# config as a variable and echoes back where it read from, for instance).
# OpenTofu writes all of them into the data source's object, so
# `.backend` reads "local" and `.config.path` reads the configured path.
data "terraform_remote_state" "rs" {
  backend = "local"
  config = {
    path = "remote.tfstate"
  }
}

output "greeting" {
  value = data.terraform_remote_state.rs.outputs.greeting
}

output "backend" {
  value = data.terraform_remote_state.rs.backend
}

output "config_path" {
  value = data.terraform_remote_state.rs.config.path
}
