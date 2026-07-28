resource "simple_resource" "c" {
  input_one = "gone-c"

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-gone'"
  }
}
