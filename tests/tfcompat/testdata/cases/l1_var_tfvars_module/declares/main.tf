variable "who" {
  type    = string
  default = "declares-default"
}

output "who" {
  value = var.who
}
