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
  name  ="src-${count.index}"
}
resource "test_item" "target" {
  name ="${test_item.source[0].name}-ref"
}
