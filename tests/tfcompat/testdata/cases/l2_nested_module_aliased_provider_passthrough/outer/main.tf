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

module "inner" {
  source = "./inner"
  tag    = var.tag
  providers = {
    simple = simple
  }
}

output "prefix_result" {
  value = module.inner.prefix_result
}
