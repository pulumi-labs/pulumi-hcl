variable "n" {
  type    = number
  default = null
}

resource "simple_resource" "r" {
  count     = var.n
  input_one = "x"
  input_two = false
}
