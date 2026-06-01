terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "source" {
  count = 2
  lifecycle {
    create_before_destroy = true
  }
  name ="src-${count.index}"
}
resource "test_item" "target" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${test_item.source[0].name}-ref"
}
