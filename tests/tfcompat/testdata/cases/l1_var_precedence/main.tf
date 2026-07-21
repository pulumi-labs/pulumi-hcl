# The order the sources of a root variable value are consulted in. Each
# variable below is set by two of them, and the higher-ranked one wins:
#
#   1. an explicitly supplied value (`-var` / Pulumi stack config)
#   2. the automatically-loaded variable-value files
#   3. TF_VAR_<name>
#   4. the declared default
variable "config_beats_tfvars" {
  type    = string
  default = "from-default"
}

variable "config_beats_env" {
  type    = string
  default = "from-default"
}

variable "tfvars_beats_env" {
  type    = string
  default = "from-default"
}

# An environment variable set to the empty string is set, so it outranks the
# default and the variable comes out empty.
variable "empty_env_beats_default" {
  type    = string
  default = "from-default"
}

output "config_beats_tfvars" { value = var.config_beats_tfvars }
output "config_beats_env" { value = var.config_beats_env }
output "tfvars_beats_env" { value = var.tfvars_beats_env }
output "empty_env_beats_default" { value = var.empty_env_beats_default }
