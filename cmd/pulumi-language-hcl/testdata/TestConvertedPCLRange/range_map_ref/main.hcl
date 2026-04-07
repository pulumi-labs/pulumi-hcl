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
  name = each.value
}
resource "test_item" "target" {
  name ="${test_item.source["x"].name}-ref"
}
