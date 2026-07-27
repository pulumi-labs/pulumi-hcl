# True only if the removed block's destroy-time provisioner ran in stage 1;
# each runtime checks its own working directory's marker.
output "provisioner_ran" {
  value = fileexists("${path.cwd}/.terraform/removed-marker")
}
