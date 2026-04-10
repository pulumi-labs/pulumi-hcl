terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_getlen" "invoke_0" {
  items = var.items
}

variable "items" {
  type = list(string)
}
locals {
  itemLen = data.test_getlen.invoke_0.result
}
output "result" {
  value = local.itemLen
}
