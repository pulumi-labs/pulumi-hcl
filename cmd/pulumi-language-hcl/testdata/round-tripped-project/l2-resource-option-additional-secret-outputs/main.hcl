terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "withSecret" {
  additional_secret_outputs = ["value"]
  value                     = true
}
resource "simple_resource" "withoutSecret" {
  value = true
}
