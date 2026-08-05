# `identity` is an Optional+Computed MaxItems=1 nested block that the provider
# leaves entirely unset at create time; `identity_set` is its TypeSet twin.
# Matching OpenTofu, the list variant materializes as null and the set variant
# stays an empty set. The terraform-provider plugin path reads the set variant
# as null too — the divergence the tfcompat case of the same name is skipped
# for.
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
