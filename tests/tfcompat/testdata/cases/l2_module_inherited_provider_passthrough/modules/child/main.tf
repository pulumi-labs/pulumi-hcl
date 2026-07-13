# child declares `simple` (like the Materialize module declares helm/kubernetes)
# but does not configure it — the config is inherited from the root default.
terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

module "gc" {
  source = "./modules/gc"
  providers = {
    simple = simple
  }
}

output "result" {
  value = module.gc.result
}
