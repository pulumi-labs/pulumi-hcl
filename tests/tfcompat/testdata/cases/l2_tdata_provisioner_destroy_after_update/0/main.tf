terraform {
  required_version = ">= 1.0"
}

# Created with one set, updated to another (stage 1), then destroyed: the
# destroy-time self.output must be the last-applied input as a set — not the
# create-time value, and not a tuple.
resource "terraform_data" "x" {
  input = toset(["a", "b"])

  provisioner "local-exec" {
    when    = destroy
    command = self.output == toset(["x", "y", "z"]) ? "true" : "false"
  }
}

output "ok" {
  value = terraform_data.x.output
}
