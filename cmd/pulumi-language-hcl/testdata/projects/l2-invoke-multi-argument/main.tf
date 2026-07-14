terraform {
  required_providers {
    multi-argument-invoke = {
      source  = "pulumi/multi-argument-invoke"
      version = "44.0.0"
    }
  }
}

output "both" {
  value = provider::multi-argument-invoke::multi_argument_invoke("hello", "world").result
}
output "onlyRequired" {
  value = provider::multi-argument-invoke::multi_argument_invoke("hello", null).result
}
