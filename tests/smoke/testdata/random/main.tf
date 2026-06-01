terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }
}

resource "random_pet" "name" {
  length = 2
}

output "pet" {
  value = random_pet.name.id
}
