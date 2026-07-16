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
