terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  input = each.value
}

resource "test_item" "inbound" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  value = data.test_echo.invoke_0.result
}
