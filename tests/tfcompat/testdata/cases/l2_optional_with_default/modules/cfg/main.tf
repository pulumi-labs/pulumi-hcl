variable "config" {
  type = object({
    name = string
    tag  = optional(string, "default-tag")
  })
}

resource "simple_resource" "r" {
  input_one = var.config.name
  input_two = false
}

output "tag" {
  value = var.config.tag
}
