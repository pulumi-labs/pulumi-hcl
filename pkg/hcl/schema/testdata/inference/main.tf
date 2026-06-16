variable "string_value" {
  type = string
}

locals {
  string_local = "${var.string_value}-app"
  number_local = 8080
}

# Outputs are typed by evaluating their value against the locals (which are
# themselves typed from the variables they reference).
output "string_output" {
  value = local.string_local
}

output "number_output" {
  value = local.number_local
}
