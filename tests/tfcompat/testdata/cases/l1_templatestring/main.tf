# `templatestring` renders a string value (here pulled from a variable) as a
# template using the supplied variables. The `$${...}` escapes keep HCL from
# interpolating the placeholders before they reach the function; the `%{ for }`
# directive exercises template control flow.
variable "greeting" {
  type    = string
  default = "Hello, $${name}! You have $${count} messages."
}

variable "list_tmpl" {
  type    = string
  default = "items:%%{ for n in names } $${n}%%{ endfor }"
}

output "simple" {
  value = templatestring(var.greeting, { name = "Ada", count = 3 })
}

output "with_directive" {
  value = templatestring(var.list_tmpl, { names = ["a", "b", "c"] })
}

output "from_literal" {
  value = templatestring("Hi $${who}", { who = "Bob" })
}
