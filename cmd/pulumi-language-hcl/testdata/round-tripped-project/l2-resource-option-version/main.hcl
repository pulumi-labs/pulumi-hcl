terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "26.0.0"
    }
  }
}

resource "simple_resource" "withV2" {
  version = "2.0.0"
  value   = true
}
resource "simple_resource" "withV26" {
  version = "26.0.0"
  value   = false
}
resource "simple_resource" "withDefault" {
  value = true
}
