terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "myItem" {
  count = true
  lifecycle {
    create_before_destroy = true
  }
  name = "static-item"
}
