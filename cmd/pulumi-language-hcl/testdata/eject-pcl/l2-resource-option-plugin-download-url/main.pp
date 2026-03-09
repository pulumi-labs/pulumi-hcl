resource "withDefaultURL" "simple:index:Resource" {
  value = true
}

resource "withExplicitDefaultURL" "simple:index:Resource" {
  value = true
}

resource "withCustomURL1" "simple:index:Resource" {
  value = true
  options {
    pluginDownloadURL = "https://custom.pulumi.test/provider1"
  }
}

resource "withCustomURL2" "simple:index:Resource" {
  value = false
  options {
    pluginDownloadURL = "https://custom.pulumi.test/provider2"
  }
}

