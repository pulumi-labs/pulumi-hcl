terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "noTimeouts" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "createOnly" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    create = "5m"
  }
  value = true
}
resource "simple_resource" "updateOnly" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    update = "10m"
  }
  value = true
}
resource "simple_resource" "deleteOnly" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    delete = "3m"
  }
  value = true
}
resource "simple_resource" "readOnly" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    read = "9m"
  }
  value = true
}
resource "simple_resource" "allTimeouts" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    create = "2m"
    update = "4m"
    delete = "1m"
    read   = "5m"
  }
  value = true
}
resource "simple_resource" "configTimeout" {
  lifecycle {
    create_before_destroy = true
  }
  timeouts {
    create = var.createTimeout
  }
  value = true
}
variable "createTimeout" {
  type = string
}
