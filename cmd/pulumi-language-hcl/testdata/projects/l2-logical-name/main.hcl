terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "aA-Alpha_alpha.🤯⁉️" {
  value = var["cC-Charlie_charlie.😃⁉️"]
}
variable "cC-Charlie_charlie.😃⁉️" {
  type = bool
}
output "bB-Beta_beta.💜⁉" {
  value = simple_resource["aA-Alpha_alpha.🤯⁉️"].value
}
output "dD-Delta_delta.🔥⁉" {
  value = simple_resource["aA-Alpha_alpha.🤯⁉️"].value
}
