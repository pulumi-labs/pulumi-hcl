terraform {
  required_providers {
    large = {
      source  = "pulumi/large"
      version = "4.3.2"
    }
  }
}

resource "large_string" "res" {
  value = "hello world"
}
output "output" {
  value = large_string.res.value
}
