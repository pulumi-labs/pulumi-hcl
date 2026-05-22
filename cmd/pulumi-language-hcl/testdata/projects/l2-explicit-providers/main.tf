terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

provider "component" {
  alias = "explicit"
}
resource "component_componentcallable" "list" {
  providers = [component.explicit]
  lifecycle {
    create_before_destroy = true
  }
  value = "value"
}
resource "component_componentcallable" "map" {
  providers = [component.explicit]
  lifecycle {
    create_before_destroy = true
  }
  value = "value"
}
