resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command = "echo this-should-be-suppressed"
    quiet   = true
  }
}
