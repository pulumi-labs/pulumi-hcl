terraform {
  required_providers {
    constant = {
      source  = "pulumi/constant"
      version = "43.0.0"
    }
  }
}

resource "constant_resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  kind = "Constant"
}
// `kind` has a constant value in the schema; reading it must bind without type errors.
output "kind" {
  value = constant_resource.first.kind
}
