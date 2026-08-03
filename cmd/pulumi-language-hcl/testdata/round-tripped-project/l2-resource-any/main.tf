terraform {
  required_providers {
    any-handled = {
      source  = "pulumi/any-handled"
      version = "42.0.0"
    }
  }
}

resource "any-handled_resource" "aString" {
  lifecycle {
    create_before_destroy = true
  }
  value = "a string"
}
resource "any-handled_resource" "aBoolean" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "any-handled_resource" "aNumber" {
  lifecycle {
    create_before_destroy = true
  }
  value = 42
}
resource "any-handled_resource" "aList" {
  lifecycle {
    create_before_destroy = true
  }
  value = [1, true, "three"]
}
resource "any-handled_resource" "anObject" {
  lifecycle {
    create_before_destroy = true
  }
  value = {
    "key" = "value"
    "nested" = {
      "count" = 1
    }
  }
}
resource "any-handled_resource" "anAsset" {
  lifecycle {
    create_before_destroy = true
  }
  value = stringasset("the asset contents")
}
