resource "random-module_random_pet" "pet" {
  length = 3
}

output "pet" {
  value = random-module_random_pet.pet.pet
}
