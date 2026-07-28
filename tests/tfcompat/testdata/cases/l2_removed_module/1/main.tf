module "m" {
  source = "./mod"
}

# The declaring module (m) still exists but no longer declares the resource.
removed {
  from = module.m.simple_resource.b

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-root'"
  }
}

# The whole module call (n) is gone along with the resource.
removed {
  from = module.n.simple_resource.c

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "mkdir -p '${path.cwd}/.terraform' && touch '${path.cwd}/.terraform/removed-marker-gone'"
  }
}
