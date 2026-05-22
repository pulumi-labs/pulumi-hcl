terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = toset(flatten([for ko, vo in {
    "a" = "alpha"
    "b" = "bravo"
  } : [vo, "${ko}-x"]]))
  input = each.value
}

output "results" {
  value = {for ko, vo in {
    "a" = "alpha"
    "b" = "bravo"
  } : ko => [for v in [vo, "${ko}-x"] : data.test_echo.invoke_0[v].result]}
}
