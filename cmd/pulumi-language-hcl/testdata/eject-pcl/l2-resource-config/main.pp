resource "prov" "pulumi:providers:config" {
  name = "my config"
  options {
    pluginDownloadURL = "not the same as the pulumi resource option"
  }
}

resource "res" "config:index:Resource" {
  text = prov.version
}

output "pluginDownloadURL" {
  value = prov.pluginDownloadURL
}

