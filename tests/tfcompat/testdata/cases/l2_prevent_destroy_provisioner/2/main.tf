# The blocked destroy must not have run the destroy-time provisioner; each
# runtime checks its own working directory's marker.
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

output "provisioner_ran" {
  value = fileexists("${path.cwd}/.terraform/pd-marker")
}
