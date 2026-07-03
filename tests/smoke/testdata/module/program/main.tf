resource "randommodule_module" "pet" {
  pet_length = 3
  object_value = {
    string_field = "hello"
  }
  map_value = {
    user_key = "world"
  }
}

output "pet_name" {
  value = randommodule_module.pet.pet_name
}

output "object_field" {
  value = randommodule_module.pet.echo_object.string_field
}

output "map_field" {
  value = randommodule_module.pet.echo_map.user_key
}

output "module_version" {
  value = randommodule_module.pet.module_version
}
