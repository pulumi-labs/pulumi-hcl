terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "res1" {
  value = true
}
output "name" {
  value = pulumiResourceName(simple_resource.res1)
}
output "type" {
  value = pulumiResourceType(simple_resource.res1)
}
