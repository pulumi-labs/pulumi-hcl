terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "noTimeouts" {
  value = true
}
resource "simple_resource" "createOnly" {
  timeouts {
    create = "5m"
  }
  value = true
}
resource "simple_resource" "updateOnly" {
  timeouts {
    update = "10m"
  }
  value = true
}
resource "simple_resource" "deleteOnly" {
  timeouts {
    delete = "3m"
  }
  value = true
}
resource "simple_resource" "allTimeouts" {
  timeouts {
    create = "2m"
    update = "4m"
    delete = "1m"
  }
  value = true
}
