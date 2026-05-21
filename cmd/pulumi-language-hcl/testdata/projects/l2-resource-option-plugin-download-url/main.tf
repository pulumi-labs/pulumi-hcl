terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "27.0.0"
    }
  }
}

resource "simple_resource" "withDefaultURL" {
  value = true
}
resource "simple_resource" "withExplicitDefaultURL" {
  value = true
}
resource "simple_resource" "withCustomURL1" {
  plugin_download_url = "https://custom.pulumi.test/provider1"
  value               = true
}
resource "simple_resource" "withCustomURL2" {
  plugin_download_url = "https://custom.pulumi.test/provider2"
  value               = false
}
