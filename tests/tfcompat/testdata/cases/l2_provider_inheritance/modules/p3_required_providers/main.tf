terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

resource "simple_resource" "r" {
  input_one = "p3"
  input_two = true
}
