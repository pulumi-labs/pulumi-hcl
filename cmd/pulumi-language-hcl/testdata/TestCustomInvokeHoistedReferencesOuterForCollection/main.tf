terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = toset(flatten([for entry in [{
    "filter" = "alpha"
    }, {
    "filter" = "bravo"
  }] : try([entry.filter], [])]))
  input = each.value
}

output "results" {
  value = [for entry in [{
    "filter" = "alpha"
    }, {
    "filter" = "bravo"
  }] : [for v in try([entry.filter], []) : data.test_echo.invoke_0[v].result]]
}
