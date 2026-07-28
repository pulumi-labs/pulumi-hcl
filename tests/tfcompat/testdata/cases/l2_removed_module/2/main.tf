module "m" {
  source = "./mod"
}

# True only if the matching removed block's destroy-time provisioner ran in
# stage 1; each runtime checks its own working directory's markers.
output "child_declared_ran" {
  value = fileexists("${path.cwd}/.terraform/removed-marker-child")
}

output "root_declared_ran" {
  value = fileexists("${path.cwd}/.terraform/removed-marker-root")
}

output "gone_module_ran" {
  value = fileexists("${path.cwd}/.terraform/removed-marker-gone")
}
