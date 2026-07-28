# A resource carrying a `when = destroy` provisioner.
resource "terraform_data" "r" {
  input = "a"

  provisioner "local-exec" {
    when = destroy
    # This should never run, if it does it will fail.
    command = "false"
  }
}
