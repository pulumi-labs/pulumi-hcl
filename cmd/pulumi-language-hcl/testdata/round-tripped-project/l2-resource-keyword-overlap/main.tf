terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "class" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "export" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "mod" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "import" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
# TODO(pulumi/pulumi#18246): Pcl should support scoping based on resource type just like HCL does in TF so we can uncomment this.
# output "import" {
#   value = Resource["import"]
# }
resource "simple_resource" "object" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "self" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "this" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "if" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
output "class" {
  value = simple_resource.class
}
output "export" {
  value = simple_resource.export
}
output "mod" {
  value = simple_resource.mod
}
output "object" {
  value = simple_resource.object
}
output "self" {
  value = simple_resource.self
}
output "this" {
  value = simple_resource.this
}
output "if" {
  value = simple_resource.if
}
