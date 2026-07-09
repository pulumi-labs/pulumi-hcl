resource "simple_resource" "target" {
  input_one = "a"
  input_two = true

  provisioner "local-exec" {
    command = "test '${self.result}' = 'a-true'"
  }
}
