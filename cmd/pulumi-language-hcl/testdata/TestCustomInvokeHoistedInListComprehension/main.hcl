pulumi {
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

output "results" {
  value = [for k, v in {
    "a" = "alpha"
    "b" = "bravo"
  } : data.test_echo.invoke_0[k].result]
}
