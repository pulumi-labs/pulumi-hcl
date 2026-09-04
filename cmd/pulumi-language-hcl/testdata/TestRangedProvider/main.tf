terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

provider "test" {
  alias = "byKey"
  for_each = {
    "a" = "alpha"
    "b" = "beta"
  }
  prefix = each.value
}
resource "test_item" "fixed" {
  provider = test.byKey["a"]
  lifecycle {
    create_before_destroy = true
  }
  value = "fixed"
}
resource "test_item" "other" {
  provider = test.byKey["b"]
  lifecycle {
    create_before_destroy = true
  }
  value = "other"
}
