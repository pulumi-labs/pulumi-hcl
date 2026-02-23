terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "withOption" {
  replace_on_changes = ["value"]
  lifecycle {
    create_before_destroy = !true
  }
  value = false
}
resource "simple_resource" "withoutOption" {
  replace_on_changes = ["value"]
  value              = false
}
