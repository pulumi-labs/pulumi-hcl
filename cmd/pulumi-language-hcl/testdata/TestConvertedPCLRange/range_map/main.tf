terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "myItem" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  lifecycle {
    create_before_destroy = true
  }
  name = each.value
}
