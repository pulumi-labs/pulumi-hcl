# Unchanged program; this stage runs a full destroy, which prevent_destroy
# refuses before the destroy-time provisioner can fire.
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
