# The marker lands under .terraform/ because that directory survives between
# stages in each runtime's working directory.
resource "simple_resource" "guarded" {
  input_one = "hello"

  lifecycle {
    prevent_destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/pd-marker'"
  }
}
