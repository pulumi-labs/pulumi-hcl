resource "simple_resource" "target" {
  input_one = "a"
  input_two = false

  provisioner "local-exec" {
    command = "true"
  }
}
