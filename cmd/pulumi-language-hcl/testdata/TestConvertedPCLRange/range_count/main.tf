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
  pulumi {
    name ="myItem-${count.index}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="item-${count.index}"
}
