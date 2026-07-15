variable "v" {
  type = string
}

output "doubled" {
  value = "${var.v}-${var.v}"
}
