terraform {
  required_providers {
    config = {
      source  = "pulumi/config"
      version = "9.0.0"
    }
  }
}

resource "pulumi_providers_config" "prov" {
  plugin_download_url = "not the same as the pulumi resource option"
  name                = "my config"
}
resource "config_resource" "res" {
  text = pulumi_providers_config.prov.version
}
output "pluginDownloadURL" {
  value = pulumi_providers_config.prov.plugin_download_url
}
