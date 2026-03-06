resource "prov" "pulumi:providers:simple" {
}

resource "res" "simple:index:Resource" {
  value = true
  options {
    provider = prov
  }
}

