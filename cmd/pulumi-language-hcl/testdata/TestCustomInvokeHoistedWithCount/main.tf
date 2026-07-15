terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  count = 2
  input ="item-${count.index}"
}

resource "test_item" "inbound" {
  count = 2
  pulumi {
    name ="inbound-${count.index}"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = data.test_echo.invoke_0[count.index].result
}
