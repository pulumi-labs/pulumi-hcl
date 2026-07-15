terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  input = each.value
}

resource "test_item" "inbound" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  pulumi {
    name ="inbound-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = data.test_echo.invoke_0[each.key].result
}
