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
  }
  input = each.value
}

output "results" {
  value = [for ko, vo in {
    "a" = "alpha"
    } : [for ki, vi in {
      "x" = "xylo"
  } : data.test_echo.invoke_0[ko].result]]
}
