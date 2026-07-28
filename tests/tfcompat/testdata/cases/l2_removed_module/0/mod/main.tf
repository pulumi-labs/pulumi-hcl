# The markers land under .terraform/ because that directory survives between
# stages in both runtimes' working directories.
resource "simple_resource" "a" {
  input_one = "child-a"

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-child'"
  }
}

resource "simple_resource" "b" {
  input_one = "child-b"

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-root'"
  }
}
