# The child module declares the removal of its own resource; the address is
# relative to this module.
removed {
  from = simple_resource.a

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-child'"
  }
}
