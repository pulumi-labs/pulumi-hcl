terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

variable "tag" {
  type = string
}

# No `providers` block: `inner` inherits this module's default `simple`.
module "inner" {
  source = "./inner"
  tag    = var.tag
}

output "prefix_result" {
  value = module.inner.prefix_result
}
