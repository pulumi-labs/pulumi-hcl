terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

module "child" {
  source = "./modules/child"
  providers = {
    simple = simple
  }
}

output "module_prefix_result" {
  value = module.child.prefix_result
}
