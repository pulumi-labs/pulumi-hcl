variable "password" {
  type = string
}

resource "simple_resource" "db" {
  input_one = var.password
}

# A value derived from the sensitive input. Its sensitivity propagates by
# derivation, so the mark must travel with it back to the calling module.
output "connection" {
  value = "conn:${var.password}"
}
