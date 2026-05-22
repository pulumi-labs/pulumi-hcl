terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "aA-Alpha_alpha.🤯⁉️" {
  lifecycle {
    create_before_destroy = true
  }
  value = var["cC-Charlie_charlie.😃⁉️"]
}
variable "cC-Charlie_charlie.😃⁉️" {
  type = bool
}
output "bB-Beta_beta.💜⁉" {
  value = simple_resource["aA-Alpha_alpha.🤯⁉️"].value
}
// New format for output logical name because outputs don't have separate logical names. Even nodejs which just
// does "export" normally for outputs needs that export _to be_ the output name and so if the "logical name"
// isn't a valid nodejs export we have to output it differently.
output "dD-Delta_delta.🔥⁉" {
  value = simple_resource["aA-Alpha_alpha.🤯⁉️"].value
}
