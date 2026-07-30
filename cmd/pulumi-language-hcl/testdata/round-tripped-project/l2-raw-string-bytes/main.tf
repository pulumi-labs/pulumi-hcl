terraform {
  required_providers {
    bytesink = {
      source  = "pulumi/bytesink"
      version = "47.0.0"
    }
    bytesource = {
      source  = "pulumi/bytesource"
      version = "48.0.0"
    }
  }
}

resource "bytesource_resource" "source" {
  lifecycle {
    create_before_destroy = true
  }
  base64 = "AGhlbGxvIID+/yB3b3JsZPAo"
}
resource "bytesink_resource" "sink" {
  lifecycle {
    create_before_destroy = true
  }
  bytes         = bytesource_resource.source.bytes
  expect_base64 = bytesource_resource.source.base64
}
