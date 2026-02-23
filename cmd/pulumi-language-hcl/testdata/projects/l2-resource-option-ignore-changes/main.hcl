terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "ignoreChanges" {
  lifecycle {
    ignore_changes = [value]
  }
  value = true
}
resource "simple_resource" "notIgnoreChanges" {
  value = true
}
