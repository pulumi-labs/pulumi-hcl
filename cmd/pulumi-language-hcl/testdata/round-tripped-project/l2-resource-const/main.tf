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
  flag = true
  _ {
    count = 3
  }
  ratio = 1.5
}
// Every property has a constant value in the schema, one per constant kind; reading them must
// bind without type errors.
output "kind" {
  value = constant_resource.first.kind
}
output "flag" {
  value = constant_resource.first.flag
}
output "count" {
  value = constant_resource.first.count
}
output "ratio" {
  value = constant_resource.first.ratio
}
