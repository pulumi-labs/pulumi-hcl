pulumi {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
  }
}

resource "pulumi_providers_component" "explicit" {
}
resource "component_component_callable" "list" {
  providers = [pulumi_providers_component.explicit]
  value     = "value"
}
resource "component_component_callable" "map" {
  providers = [pulumi_providers_component.explicit]
  value     = "value"
}
