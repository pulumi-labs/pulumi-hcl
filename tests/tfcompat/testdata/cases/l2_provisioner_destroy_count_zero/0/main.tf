resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "exit 7"
  }
}
