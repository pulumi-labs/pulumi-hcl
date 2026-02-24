terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "class" {
  value = true
}
resource "simple_resource" "export" {
  value = true
}
resource "simple_resource" "mod" {
  value = true
}
resource "simple_resource" "import" {
  value = true
}
resource "simple_resource" "object" {
  value = true
}
resource "simple_resource" "self" {
  value = true
}
resource "simple_resource" "this" {
  value = true
}
resource "simple_resource" "if" {
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
