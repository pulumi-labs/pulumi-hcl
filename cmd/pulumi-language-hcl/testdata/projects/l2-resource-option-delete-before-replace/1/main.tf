terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

// Stage 1: Change properties to trigger replacements
// Resource with deleteBeforeReplace option - should delete before creating
resource "simple_resource" "withOption" {
  pulumi {
    replace_on_changes = ["value"]
  }
  value = false // Changed to trigger replacement
}
// Resource without deleteBeforeReplace - should create before deleting (default)
resource "simple_resource" "withoutOption" {
  pulumi {
    replace_on_changes = ["value"]
  }
  lifecycle {
    create_before_destroy = true
  }
  value = false // Changed to trigger replacement
}
