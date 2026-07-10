terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

resource "simple_resource" "r" {
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
