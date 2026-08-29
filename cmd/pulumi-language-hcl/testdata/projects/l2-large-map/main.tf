terraform {
  required_providers {
    large = {
      source  = "pulumi/large"
      version = "4.3.2"
    }
  }
}

resource "large_map" "res" {
  lifecycle {
    create_before_destroy = true
  }
  value = "leaf"
  depth = 300
}
output "output" {
  value = large_map.res.value
}
