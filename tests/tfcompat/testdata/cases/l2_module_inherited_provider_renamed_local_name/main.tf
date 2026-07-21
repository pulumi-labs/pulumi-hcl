terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

provider "simple" {
  prefix = "root"
}

module "child" {
  source = "./modules/child"
}

output "child_prefix" {
  value = module.child.prefix_result
}
