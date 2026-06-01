terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "example" {
  input = "hello"
}

output "value" {
  value = terraform_data.example.output
}

output "input_echo" {
  value = terraform_data.example.input
}
