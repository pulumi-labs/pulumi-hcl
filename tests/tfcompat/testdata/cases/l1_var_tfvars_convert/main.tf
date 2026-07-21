# A `terraform.tfvars` value that cannot convert to the variable's declared
# type is an error.
variable "n" {
  type = number
}

output "n" {
  value = var.n
}
