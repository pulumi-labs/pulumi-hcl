terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "myConfig" {
  input = "hello"
}

output "result" {
  value = data.test_echo.myConfig.result
}
