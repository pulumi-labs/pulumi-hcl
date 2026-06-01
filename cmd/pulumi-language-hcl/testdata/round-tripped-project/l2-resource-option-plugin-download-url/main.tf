terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "27.0.0"
    }
  }
}

resource "simple_resource" "withDefaultURL" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "withExplicitDefaultURL" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "withCustomURL1" {
  pulumi {
    plugin_download_url = "https://custom.pulumi.test/provider1"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "withCustomURL2" {
  pulumi {
    plugin_download_url = "https://custom.pulumi.test/provider2"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
