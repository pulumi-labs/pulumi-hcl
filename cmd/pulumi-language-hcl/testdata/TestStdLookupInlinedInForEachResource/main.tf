terraform {
  required_providers {
    std = {
      source  = "pulumi/std"
      version = "1.0.0"
    }
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "inbound" {
  for_each = {
    "a" = {
      "thing" = "alpha"
    }
    "b" = {
      "thing" = "bravo"
    }
  }
  lifecycle {
    create_before_destroy = true
  }
  value = lookup(each.value, "thing", "none")
}
