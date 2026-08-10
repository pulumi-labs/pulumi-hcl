variable "name" {
  type = string
}

output "greeting" {
  value = "hello ${var.name}"
}
