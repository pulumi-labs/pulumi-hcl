terraform {
  required_providers {
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "~> 3.9"
    }
  }
}

module "pet" {
  source = "./pet"
}

output "pet" {
  value = module.pet.pet
}
