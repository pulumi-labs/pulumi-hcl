terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.5"
    }
  }
}

# A file bundled with the module, read through a bridged provider data source
# (https://github.com/pulumi-labs/pulumi-hcl/issues/305).
data "local_file" "version" {
  filename = "${path.module}/VERSION"
}

# Multi-word and nested names exercise the snake_case<->camelCase translation at
# the component boundary: the schema exposes camelCase, the HCL module is
# snake_case. Object fields are renamed; the dynamic keys of a map are not.
variable "pet_length" {
  type = number
}

variable "object_value" {
  type = object({
    string_field = string
  })
}

variable "map_value" {
  type = map(string)
}

resource "random_pet" "name" {
  length = var.pet_length
}

output "pet_name" {
  value = random_pet.name.id
}

output "echo_object" {
  value = var.object_value
}

output "echo_map" {
  value = var.map_value
}

output "module_version" {
  value = trimspace(data.local_file.version.content)
}
