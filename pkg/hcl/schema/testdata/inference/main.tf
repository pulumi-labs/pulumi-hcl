variable "region" {
  type = string
}

locals {
  prefix = "${var.region}-app"
  port   = 8080
}

# Outputs are typed by evaluating their value against the locals (which are
# themselves typed from the variables they reference).
output "prefix" {
  value = local.prefix
}

output "port" {
  value = local.port
}
