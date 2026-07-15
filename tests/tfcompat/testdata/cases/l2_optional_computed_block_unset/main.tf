# `identity` is an Optional+Computed MaxItems=1 nested block that the provider
# leaves entirely unset at create time. In OpenTofu an unset optional+computed
# singular block materializes as null, so `== null` is true and jsonencode
# yields "null". pulumi-hcl materializes it as an empty list [] instead, so the
# guard flips and downstream `identity[0]` indexing no longer errors the way a
# migrated OpenTofu program expects.
resource "optcomp_thing" "t" {
  name = "probe"
}

output "identity_is_null" {
  value = optcomp_thing.t.identity == null
}

output "identity_json" {
  value = jsonencode(optcomp_thing.t.identity)
}
