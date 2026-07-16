terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "x" {
  input = { k = "v1", other = "keep" }

  lifecycle {
    ignore_changes = [input["k"]]
  }
}

output "k" {
  value = terraform_data.x.output.k
}

output "other" {
  value = terraform_data.x.output.other
}
