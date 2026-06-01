terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

# Check that withV2 is generated against the v2 SDK and not against the V26 SDK,
# and that the version resource option is elided.
resource "simple_resource" "withV2" {
  pulumi {
    version = "2.0.0"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
