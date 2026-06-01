terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "targetOnly" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "unrelated" {
  depends_on = [simple_resource.dep]
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "dep" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
