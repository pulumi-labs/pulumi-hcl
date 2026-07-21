# A resource carrying a `when = destroy` provisioner.
resource "terraform_data" "r" {
  input = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}
