terraform {
  required_providers {
    myp = {
      source = "hashicorp/simple"
    }
  }
}

resource "simple_resource" "r" {
  provider  = myp
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
