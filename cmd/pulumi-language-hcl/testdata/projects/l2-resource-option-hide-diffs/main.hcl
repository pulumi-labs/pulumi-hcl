terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "hideDiffs" {
  hide_diffs = ["value"]
  value      = true
}
resource "simple_resource" "notHideDiffs" {
  value = true
}
