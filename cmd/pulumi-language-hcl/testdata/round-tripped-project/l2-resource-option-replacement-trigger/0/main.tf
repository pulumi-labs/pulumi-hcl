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
    replace_triggered_by  = ["test"]
    create_before_destroy = true
  }
  value = true
}
resource "output_resource" "unknown" {
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
resource "simple_resource" "unknownReplacementTrigger" {
  lifecycle {
    replace_triggered_by  = ["hellohello"]
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
    replace_triggered_by  = [sensitive([1, 2, 3])]
    create_before_destroy = true
  }
  value = true
}
