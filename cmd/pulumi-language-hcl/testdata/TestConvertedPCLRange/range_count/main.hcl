terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "myItem" {
  count = 3
  name  ="item-${count.index}"
}
