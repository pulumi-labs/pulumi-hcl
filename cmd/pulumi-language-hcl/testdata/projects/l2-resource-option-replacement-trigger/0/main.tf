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
  replacement_trigger = "test"
  lifecycle {
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
  replacement_trigger = "hellohello"
  lifecycle {
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
  replacement_trigger = sensitive([1, 2, 3])
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
