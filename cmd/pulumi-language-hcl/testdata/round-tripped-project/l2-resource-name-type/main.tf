terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "res1" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
output "name" {
  value = pulumiresourcename(simple_resource.res1)
}
output "type" {
  value = pulumiresourcetype(simple_resource.res1)
}
