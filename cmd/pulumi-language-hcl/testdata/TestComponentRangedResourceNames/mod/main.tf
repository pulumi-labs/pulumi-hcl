terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "keyed" {
  for_each = {
    "x" = "alpha"
    "y" = "bravo"
  }
  pulumi {
    name ="${pulumi.module.name}-keyed-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name = each.value
}
resource "test_item" "counted" {
  count = 2
  pulumi {
    name ="${pulumi.module.name}-counted-${count.index}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="item-${count.index}"
}
