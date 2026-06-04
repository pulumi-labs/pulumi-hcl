terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "example" {
  input = "hello"
}

# input is optional in Terraform: a terraform_data used only for its
# triggers_replace lifecycle omits it, leaving input/output null.
resource "terraform_data" "no_input" {
  triggers_replace = ["v1"]
}

output "value" {
  value = terraform_data.example.output
}

output "input_echo" {
  value = terraform_data.example.input
}

output "no_input_output_null" {
  value = terraform_data.no_input.output == null
}

output "no_input_input_null" {
  value = terraform_data.no_input.input == null
}

output "no_input_triggers" {
  value = terraform_data.no_input.triggers_replace
}
