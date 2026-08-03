resource "random_pet" "this" {
  length = 2
}

output "pet" {
  value = random_pet.this.id
}
