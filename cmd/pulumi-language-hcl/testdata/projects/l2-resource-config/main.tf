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
  name                = "my config"
  plugin_download_url = "not the same as the pulumi resource option"
}
// Note this isn't _using_ the explicit provider, it's just grabbing a value from it.
resource "config_resource" "res" {
  text = config.prov.version
}
output "pluginDownloadURL" {
  value = config.prov.plugin_download_url
}
