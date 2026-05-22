terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "target" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "replaceWith" {
  replace_with = [simple_resource.target]
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "notReplaceWith" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
