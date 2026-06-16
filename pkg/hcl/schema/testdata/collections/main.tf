variable "map_of_string" {
  type    = map(string)
  default = {}
}

variable "list_of_string" {
  type    = list(string)
  default = []
}

variable "set_of_number" {
  type    = set(number)
  default = []
}

variable "object_value" {
  type = object({
    string_field = string
    bool_field   = bool
  })
  default = {
    string_field = "10.0.0.0/16"
    bool_field   = false
  }
}

output "list_of_string_output" {
  value = var.list_of_string
}

output "map_of_string_output" {
  value = var.map_of_string
}
