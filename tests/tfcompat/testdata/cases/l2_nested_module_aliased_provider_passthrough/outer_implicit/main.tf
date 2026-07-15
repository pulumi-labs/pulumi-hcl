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

# No `providers` block: `inner` inherits this module's default `simple`, which
# was itself passed in from the parent.
module "inner" {
  source = "./inner"
  tag    = var.tag
}

output "prefix_result" {
  value = module.inner.prefix_result
}
