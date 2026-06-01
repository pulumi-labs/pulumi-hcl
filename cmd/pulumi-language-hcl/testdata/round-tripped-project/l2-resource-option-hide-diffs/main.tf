terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "hideDiffs" {
  pulumi {
    hide_diffs = ["value"]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "notHideDiffs" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
