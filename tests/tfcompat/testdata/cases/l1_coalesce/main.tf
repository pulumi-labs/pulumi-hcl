# Terraform's `coalesce` returns the first argument that is neither null nor an
# empty string. Empty strings are skipped, while other "zero" values such as the
# number 0 are values in their own right and are returned as-is.
output "empty_first" {
  value = coalesce("", "b")
}

output "all_empty_but_last" {
  value = coalesce("", "", "x")
}

output "first_nonempty" {
  value = coalesce("a", "b")
}

output "null_skipped" {
  value = coalesce(null, "z")
}

output "null_then_empty" {
  value = coalesce(null, "", "w")
}

output "zero_is_value" {
  value = coalesce(0, 5)
}

# Empty collections are values in their own right: only null and empty strings
# are skipped, so an empty list or map is returned as-is.
output "empty_list" {
  value = coalesce(tolist([]), tolist(["y"]))
}

output "nonempty_list" {
  value = coalesce(tolist(["a"]), tolist(["y"]))
}

output "empty_map" {
  value = coalesce(tomap({}), tomap({ k = "v" }))
}

output "nonempty_map" {
  value = coalesce(tomap({ a = "b" }), tomap({ k = "v" }))
}
