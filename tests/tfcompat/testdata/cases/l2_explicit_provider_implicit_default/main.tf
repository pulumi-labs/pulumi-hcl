terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

resource "simple_resource" "a" {
  provider  = simple
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.a.prefix_result
}
