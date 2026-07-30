resource "simple_resource" "guarded" {
  count     = 2
  input_one = "n-${count.index}"

  lifecycle {
    prevent_destroy = true
  }
}
