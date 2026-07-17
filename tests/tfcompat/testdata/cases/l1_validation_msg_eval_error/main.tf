variable "other" {
  type     = string
  nullable = true
  default  = null
}

variable "name" {
  type    = string
  default = "ab"

  # The condition fails (name is 2 chars). Rendering error_message then fails
  # too, because interpolating a null value into a string template is an error.
  # OpenTofu surfaces that template-interpolation error; the question is whether
  # pulumi-hcl does the same or swallows it behind a generic message.
  validation {
    condition     = length(var.name) > 5
    error_message = "name too short (ctx: ${var.other})"
  }
}

output "name" {
  value = var.name
}
