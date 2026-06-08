terraform {
  component {
    name = "RandomPet"
  }
  package {
    name = "random-module"
  }
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }
}

variable "length" {
  type = number
}

resource "random_pet" "name" {
  length = var.length
}

output "pet" {
  value = random_pet.name.id
}
