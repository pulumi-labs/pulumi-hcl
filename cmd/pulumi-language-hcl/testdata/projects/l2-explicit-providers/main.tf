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
resource "component_component_callable" "list" {
  providers = [component.explicit]
  lifecycle {
    create_before_destroy = true
  }
  value = "value"
}
resource "component_component_callable" "map" {
  providers = [component.explicit]
  lifecycle {
    create_before_destroy = true
  }
  value = "value"
}
