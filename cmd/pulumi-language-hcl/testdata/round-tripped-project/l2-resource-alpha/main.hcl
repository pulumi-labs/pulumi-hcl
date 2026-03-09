terraform {
  required_providers {
    alpha = {
      source  = "pulumi/alpha"
      version = "3.0.0-alpha.1.internal+exp.sha.12345678"
    }
  }
}

resource "alpha_resource" "res" {
  value = true
}
