resource "simple_resource" "a" {
  input_one = "x"
  input_two = false
}

resource "simple_resource" "b" {
  input_one = "y"
  input_two = false

  lifecycle {
    prevent_destroy = simple_resource.a.result != ""
  }
}
