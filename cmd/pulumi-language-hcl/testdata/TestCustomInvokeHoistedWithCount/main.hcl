pulumi {
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
  value = data.test_echo.invoke_0[count.index].result
}
