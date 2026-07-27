resource "simple_resource" "target" {
  count     = 0
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "exit 7"
  }
}
