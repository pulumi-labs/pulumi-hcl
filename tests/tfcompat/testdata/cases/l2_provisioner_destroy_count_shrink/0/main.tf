resource "simple_resource" "target" {
  count     = 2
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "exit 7"
  }
}
