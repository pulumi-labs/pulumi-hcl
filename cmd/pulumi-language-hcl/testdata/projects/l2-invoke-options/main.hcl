terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_myinvoke" "invoke_0" {
  value               = "hello"
  provider            = pulumi_providers_simple-invoke.explicitProvider
  parent              = pulumi_providers_simple-invoke.explicitProvider
  version             = "10.0.0"
  plugin_download_url = "https://example.com/github/example"
}

resource "pulumi_providers_simple-invoke" "explicitProvider" {
}
locals {
  data = data.simple-invoke_myinvoke.invoke_0
}
output "hello" {
  value = local.data.result
}
