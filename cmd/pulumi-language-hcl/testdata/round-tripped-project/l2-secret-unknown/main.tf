terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
  }
}

resource "output_resource" "r" {
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
# During preview `r.output` resolves to unknown. Wrapping an unknown value with
# secret() must preserve the secret marker when the stack output is serialised
# back to the engine.
output "wrapped" {
  value = sensitive(output_resource.r.output)
}
