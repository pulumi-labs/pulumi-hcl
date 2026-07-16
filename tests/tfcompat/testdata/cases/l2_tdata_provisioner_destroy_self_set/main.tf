terraform {
  required_version = ">= 1.0"
}

# A destroy-time provisioner's `self` comes from prior state, where OpenTofu
# stores terraform_data's dynamically-typed output together with its cty type:
# the command exits 0 only when the set equality holds, so a dropped set type
# fails the destroy.
resource "terraform_data" "x" {
  input = toset(["a", "b", "c"])

  provisioner "local-exec" {
    when    = destroy
    command = self.output == toset(["a", "b", "c"]) ? "true" : "false"
  }
}

output "ok" {
  value = terraform_data.x.output
}
