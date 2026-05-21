terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "targetOnly" {
  value = true
}
resource "simple_resource" "unrelated" {
  value = true
}
