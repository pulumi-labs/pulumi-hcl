terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_my_invoke" "invoke_0" {
  value    = "hello"
  provider = simple-invoke.explicitProvider
  pulumi {
    parent              = simple-invoke.explicitProvider
    version             = "10.0.0"
    plugin_download_url = "https://example.com/github/example"
  }
}

provider "simple-invoke" {
  alias = "explicitProvider"
}
output "hello" {
  value = data.simple-invoke_my_invoke.invoke_0.result
}
