terraform {
  component {
    name = "BuiltinInfo"
  }
  package {
    name    = "builtin-info-component"
    version = "37.0.0"
  }
}

output "organization" {
  value = pulumi.organization
}

output "project" {
  value = pulumi.project
}

output "stack" {
  value = pulumi.stack
}
