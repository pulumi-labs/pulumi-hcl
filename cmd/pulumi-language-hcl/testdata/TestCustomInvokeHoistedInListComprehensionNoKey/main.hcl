pulumi {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = toset(["alpha", "bravo"])
  input    = each.value
}

output "results" {
  value = [for v in ["alpha", "bravo"] : data.test_echo.invoke_0[v].result]
}
