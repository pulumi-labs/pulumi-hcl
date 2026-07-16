terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "x" {
  input = toset(["x", "y", "z"])

  provisioner "local-exec" {
    when    = destroy
    command = self.output == toset(["x", "y", "z"]) ? "true" : "false"
  }
}

output "ok" {
  value = terraform_data.x.output
}
