resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command = "exit 1"
  }
}
