variable "in" {
  type    = string
  default = "hunter2"
}

output "token" {
  value     = var.in
  sensitive = true
}
