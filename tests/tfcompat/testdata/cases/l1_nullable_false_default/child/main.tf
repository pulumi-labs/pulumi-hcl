# Assigning `null` to a `nullable = false` variable must substitute the
# variable's default, so `length(var.items)` sees ["a", "b"] rather than null.
variable "items" {
  type     = list(string)
  default  = ["a", "b"]
  nullable = false
}

# Only null triggers default substitution. An empty string is a real value, so
# `var.name` stays "" rather than falling back to the default — unlike coalesce,
# which skips empty strings.
variable "name" {
  type     = string
  default  = "default-name"
  nullable = false
}

output "count" {
  value = length(var.items)
}

output "name" {
  value = var.name
}
