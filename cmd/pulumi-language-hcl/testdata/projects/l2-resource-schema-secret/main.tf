terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
  }
}

provider "output" {
  alias          = "provElided"
  elide_unknowns = true
}
provider "output" {
  alias = "provNotElided"
}
resource "output_resource" "topLevelElided" {
  provider = output.provElided
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
resource "output_resource" "topLevelNotElided" {
  provider = output.provNotElided
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
output "topLevelElided" {
  value = output_resource.topLevelElided.secret_output
}
output "topLevelNotElided" {
  value = output_resource.topLevelNotElided.secret_output
}
