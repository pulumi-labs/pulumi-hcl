terraform {
  required_providers {
    config = {
      source  = "pulumi/config"
      version = "9.0.0"
    }
  }
}

provider "config" {
  alias               = "prov"
  plugin_download_url = "not the same as the pulumi resource option"
  name                = "my config"
}
// Note this isn't _using_ the explicit provider, it's just grabbing a value from it.
resource "config_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  text = config.prov.version
}
output "pluginDownloadURL" {
  value = config.prov.plugin_download_url
}
