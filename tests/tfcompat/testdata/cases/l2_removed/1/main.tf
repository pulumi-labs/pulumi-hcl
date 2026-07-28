removed {
  from = simple_resource.a

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker'"
  }
}

removed {
  from = simple_resource.b

  lifecycle {
    destroy = true
  }
}
