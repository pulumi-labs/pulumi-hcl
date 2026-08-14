resource "simple_resource" "r" {
  input_one = "x"
  input_two = false

  lifecycle {
    prevent_destroy = null
  }
}
