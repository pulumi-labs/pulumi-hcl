terraform {
  required_version = ">= 1.0"
}

# A provisioner's `self` must see terraform_data's output still typed as a set:
# the command exits 0 only when the set equality holds, so a dropped set type
# fails the apply.
resource "terraform_data" "x" {
  input = toset(["a", "b", "c"])

  provisioner "local-exec" {
    command = self.output == toset(["a", "b", "c"]) ? "true" : "false"
  }
}

output "ok" {
  value = terraform_data.x.output
}
