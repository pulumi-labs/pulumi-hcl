variable "name" {
  type = string
}

output "content" { value = file("~/${var.name}") }
output "b64"     { value = filebase64("~/${var.name}") }
output "exists"  { value = fileexists("~/${var.name}") }
