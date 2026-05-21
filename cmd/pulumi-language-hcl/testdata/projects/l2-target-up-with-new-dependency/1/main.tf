terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "targetOnly" {
  value = true
}
resource "simple_resource" "unrelated" {
  depends_on = [simple_resource.dep]
  value      = true
}
resource "simple_resource" "dep" {
  value = true
}
