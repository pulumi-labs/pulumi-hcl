terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "noDependsOn" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "withDependsOn" {
  depends_on = [simple_resource.noDependsOn]
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
