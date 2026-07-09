resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "test -n '${self.id}'"
  }
}
