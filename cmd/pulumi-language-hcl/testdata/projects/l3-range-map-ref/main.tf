terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_target" "mapResource" {
  for_each = var.itemMap
  pulumi {
    name ="mapResource-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="${each.key}=${each.value}"
}
resource "nestedobject_target" "mapTarget" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${nestedobject_target.mapResource["k1"].name}+"
}
variable "itemMap" {
  type = map(string)
}
