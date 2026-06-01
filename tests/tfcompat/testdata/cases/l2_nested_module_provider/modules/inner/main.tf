terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

variable "marker" { type = string }

provider "simple" {
  alias  = "alpha"
  prefix = "alpha"
}

resource "simple_resource" "r" {
  provider  = simple.alpha
  input_one = var.marker
}

output "marker" {
  value = simple_resource.r.prefix_result
}
