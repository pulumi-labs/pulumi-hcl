terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "noDependsOn" {
  value = true
}
resource "simple_resource" "withDependsOn" {
  depends_on = [simple_resource.noDependsOn]
  value      = false
}
