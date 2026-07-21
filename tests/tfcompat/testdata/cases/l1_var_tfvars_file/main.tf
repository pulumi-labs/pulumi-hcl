# `terraform.tfvars` in the program directory is loaded automatically and
# overrides the variable's default.
variable "greeting" {
  type    = string
  default = "from-default"
}

output "greeting" {
  value = var.greeting
}
