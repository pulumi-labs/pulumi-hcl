# The automatically-loaded variable-value files are applied in order:
# `terraform.tfvars`, then `terraform.tfvars.json`, then every `*.auto.tfvars`
# and `*.auto.tfvars.json` interleaved lexically by file name. The last file to
# set a name wins.
variable "a" { type = string }
variable "b" { type = string }
variable "c" { type = string }
variable "d" { type = string }

output "a" { value = var.a }
output "b" { value = var.b }
output "c" { value = var.c }
output "d" { value = var.d }
