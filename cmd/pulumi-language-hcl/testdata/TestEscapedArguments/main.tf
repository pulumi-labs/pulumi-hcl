terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  _ {
    provider = "escaped-provider"
  }
}

resource "test_item" "escaped" {
  lifecycle {
    create_before_destroy = true
  }
  _ {
    count     = 3
    lifecycle = "not-a-block"
  }
}
module "mod" {
  source = "./mod"
  _ {
    source = "escaped-source"
  }
}
output "result" {
  value = data.test_echo.invoke_0.result
}
