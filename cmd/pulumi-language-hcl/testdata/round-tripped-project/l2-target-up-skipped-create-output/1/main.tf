terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "target" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "nestedobject_container" "other" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = ["a"]
}
resource "nestedobject_container" "skipped" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = ["b"]
}
output "skippedOutput" {
  value ="skipped-${nestedobject_container.skipped.details[0].key}"
}
