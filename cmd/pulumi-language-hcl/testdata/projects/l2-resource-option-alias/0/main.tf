terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

// Make a simple resource to use as a parent
resource "simple_resource" "parent" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasURN" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasName" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasNoParent" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "aliasParent" {
  pulumi {
    parent = simple_resource.aliasURN
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "component_custom" "aliasType" {
  lifecycle {
    create_before_destroy = true
  }
  value = "true"
}
