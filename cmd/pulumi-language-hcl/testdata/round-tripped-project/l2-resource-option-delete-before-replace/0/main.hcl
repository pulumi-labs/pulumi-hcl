terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

// Stage 0: Initial resource creation
// Resource with deleteBeforeReplace option
resource "simple_resource" "withOption" {
  replace_on_changes = ["value"]
  lifecycle {
    create_before_destroy = !true
  }
  value = true
}
// Resource without deleteBeforeReplace (default create-before-delete behavior)
resource "simple_resource" "withoutOption" {
  replace_on_changes = ["value"]
  value              = true
}
