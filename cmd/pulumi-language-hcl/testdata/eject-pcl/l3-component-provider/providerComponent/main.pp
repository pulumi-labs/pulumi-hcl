resource "prov" "pulumi:providers:config" {
  name = "my config"
}

resource "res" "config:index:Resource" {
  text = text
  options {
    provider = prov
  }
}

config "text" "string" {
}

output "result" {
  value = res.text
}

