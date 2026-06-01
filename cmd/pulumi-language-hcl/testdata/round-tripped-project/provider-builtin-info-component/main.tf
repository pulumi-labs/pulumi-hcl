terraform {
  required_providers {
    builtin-info-component = {
      source  = "pulumi/builtin-info-component"
      version = "37.0.0"
    }
  }
}

resource "builtin-info-component_builtin_info" "res" {
  lifecycle {
    create_before_destroy = true
  }
}
