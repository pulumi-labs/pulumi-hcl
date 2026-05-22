terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "source" {
  for_each = {
    "x" = "alpha"
    "y" = "bravo"
  }
  lifecycle {
    create_before_destroy = true
  }
  name = each.value
}
resource "test_item" "target" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${test_item.source["x"].name}-ref"
}
