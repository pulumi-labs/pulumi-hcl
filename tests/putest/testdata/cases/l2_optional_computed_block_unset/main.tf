# `identity` is an Optional+Computed MaxItems=1 nested block that the provider
# leaves entirely unset at create time; `identity_set` is its TypeSet twin.
# Through the dynamic bridge both materialize as null, so the `== null` guards
# hold and jsonencode yields "null" for each. (OpenTofu keeps the set twin an
# empty set — see the skipped tfcompat case of the same name.)
resource "optcomp_thing" "t" {
  name = "probe"
}

output "identity_is_null" {
  value = optcomp_thing.t.identity == null
}

output "identity_json" {
  value = jsonencode(optcomp_thing.t.identity)
}

output "identity_set_is_null" {
  value = optcomp_thing.t.identity_set == null
}

output "identity_set_json" {
  value = jsonencode(optcomp_thing.t.identity_set)
}
