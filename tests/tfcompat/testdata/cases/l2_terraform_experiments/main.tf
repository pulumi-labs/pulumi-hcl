# `experiments = [module_variable_optional_attrs]` is a graduated experiment
# opt-in that OpenTofu still accepts for backward compatibility with configs
# written during the experiment period. It applies cleanly (no warning, no
# error). Real Terraform 0.14-era configs still carry this line.
terraform {
  experiments = [module_variable_optional_attrs]
}

output "ok" { value = "hello" }
