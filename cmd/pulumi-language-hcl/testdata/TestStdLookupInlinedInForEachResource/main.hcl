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

data "std_lookup" "invoke_0" {
  map     = each.value
  key     = "thing"
  default = "none"
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
  value = data.std_lookup.invoke_0.result
}
