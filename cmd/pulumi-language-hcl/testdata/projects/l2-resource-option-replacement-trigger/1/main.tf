terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "replacementTrigger" {
  lifecycle {
    replace_triggered_by  = ["test2"]
    create_before_destroy = true
  }
  value = true
}
resource "output_resource" "unknown" {
  lifecycle {
    create_before_destroy = true
  }
  value = 2
}
resource "simple_resource" "unknownReplacementTrigger" {
  lifecycle {
    replace_triggered_by  = [output_resource.unknown.output]
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "notReplacementTrigger" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "secretReplacementTrigger" {
  lifecycle {
    replace_triggered_by  = [sensitive([3, 2, 1])]
    create_before_destroy = true
  }
  value = true
}
