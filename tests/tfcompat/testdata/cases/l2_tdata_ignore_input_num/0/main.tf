terraform {
  required_version = ">= 1.0"
}
resource "terraform_data" "x" {
  input = ["v1", "keep"]
  lifecycle {
    ignore_changes = [input[0]]
  }
}
output "first" { value = terraform_data.x.output[0] }
