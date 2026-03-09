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
  replacement_trigger = "test2"
  value               = true
}
resource "output_resource" "unknown" {
  value = 2
}
resource "simple_resource" "unknownReplacementTrigger" {
  replacement_trigger = output_resource.unknown.output
  value               = true
}
resource "simple_resource" "notReplacementTrigger" {
  value = true
}
resource "simple_resource" "secretReplacementTrigger" {
  replacement_trigger = sensitive([3, 2, 1])
  value               = true
}
