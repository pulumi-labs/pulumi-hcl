terraform {
  required_providers {
    fail_on_create = {
      source  = "pulumi/fail_on_create"
      version = "4.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "fail_on_create_resource" "failing" {
  value = false
}
resource "simple_resource" "dependent" {
  depends_on = [fail_on_create_resource.failing]
  value      = true
}
resource "simple_resource" "dependent_on_output" {
  value = fail_on_create_resource.failing.value
}
resource "simple_resource" "independent" {
  value = true
}
resource "simple_resource" "double_dependency" {
  depends_on = [simple_resource.independent, simple_resource.dependent_on_output]
  value      = true
}
