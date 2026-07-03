terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_info" "invoke_0" {
}

resource "test_sink" "targets" {
  count = length(data.test_info.invoke_0.items)
  lifecycle {
    create_before_destroy = true
  }
  value = "fixed"
}
