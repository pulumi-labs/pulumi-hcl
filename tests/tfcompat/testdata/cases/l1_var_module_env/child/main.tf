variable "who" {
  type    = string
  default = "child-default"
}

output "who" {
  value = var.who
}
