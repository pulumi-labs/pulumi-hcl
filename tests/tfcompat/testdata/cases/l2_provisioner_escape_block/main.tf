# OpenTofu supports the special `_` escaping block inside a provisioner, whose
# attributes are merged into the provisioner's configuration (it exists to let
# provisioner arguments that collide with meta-argument names be written
# unambiguously). Here `command` is supplied through the escaping block; on
# OpenTofu the merge makes it the provisioner's command, so `true` runs and the
# apply succeeds.
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    _ {
      command = "true"
    }
  }
}

# Resource (and data/provider/module) blocks accept the same escaping block.
# `input_one` written inside `_` must land in the resource's configuration, and
# `for_each` — a resource attribute whose name collides with the meta-argument
# — can only be set through the escaping block.
resource "simple_resource" "escaped" {
  lifecycle {
    prevent_destroy = false
  }

  _ {
    input_one = simple_resource.target.result
    for_each  = "not-the-meta-argument"
  }
}
