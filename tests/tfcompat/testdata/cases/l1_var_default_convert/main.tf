# A `default` that does not fit the variable's declared type is an error, just
# like a supplied value that does not fit.
variable "n" {
  type    = number
  default = "abc"
}

output "n" {
  value = var.n
}
