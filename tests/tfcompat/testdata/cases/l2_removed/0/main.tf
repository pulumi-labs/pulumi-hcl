# The marker lands under .terraform/ because that directory survives between
# stages in both runtimes' working directories.
resource "simple_resource" "a" {
  input_one = "plain-a"

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker'"
  }
}

resource "simple_resource" "b" {
  input_one = "plain-b"
}
