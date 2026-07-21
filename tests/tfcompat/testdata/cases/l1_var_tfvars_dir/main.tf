# `terraform.tfvars` here is a directory, not a file. OpenTofu stats the name,
# finds it, and fails to read it.
variable "greeting" {
  type    = string
  default = "from-default"
}

output "greeting" {
  value = var.greeting
}
