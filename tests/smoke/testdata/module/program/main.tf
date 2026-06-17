resource "randommodule" "pet" {
  pet_length = 3
  object_value = {
    string_field = "hello"
  }
  map_value = {
    user_key = "world"
  }
}

output "pet_name" {
  value = randommodule.pet.pet_name
}

output "object_field" {
  value = randommodule.pet.echo_object.string_field
}

output "map_field" {
  value = randommodule.pet.echo_map.user_key
}
