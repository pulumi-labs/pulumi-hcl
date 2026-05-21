terraform {
  required_providers {
    smoketest = {
      source  = "pulumi/smoketest"
      version = "1.0.0"
    }
  }
}

resource "smoketest_echo" "myecho" {
  value = "hello"
}
