resource "randommodule" "pet" {
  length = 3
}

output "pet" {
  value = randommodule.pet.pet
}
