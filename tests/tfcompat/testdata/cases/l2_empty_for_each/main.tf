# Resources whose `for_each` / `count` evaluate to empty (analogous to the
# aws-ia/vpc `aws_route_table.tgw` block whose `for_each` is gated on the
# user opting into a transit_gateway subnet). The resource address must
# still be a valid reference returning an empty collection — which is what
# the surrounding module output relies on.
resource "simple_resource" "for_each_empty" {
  for_each  = false ? toset(["a", "b"]) : toset([])
  input_one = each.key
  input_two = false
}

resource "simple_resource" "count_zero" {
  count     = 0
  input_one = "ignored"
  input_two = false
}

# Empty set produced by a comprehension over an empty input (analogous to
# aws-ia/vpc's `toset(local.ipv6_private_subnet_key_names_tgw_routed)` when
# the user opts out of TGW routes). The set element type is `dynamic`, not
# `string`, but the set is empty so for_each should still be a no-op.
locals {
  filtered_empty = [for x in [] : x]
}

resource "simple_resource" "for_each_empty_dynamic" {
  for_each  = toset(local.filtered_empty)
  input_one = each.key
  input_two = false
}

# Force at least one resource creation so both runtimes have a non-empty
# stack (some preview-only paths short-circuit on no-op stacks).
resource "simple_resource" "anchor" {
  input_one = "anchor"
  input_two = false
}

output "for_each_empty_map" {
  value = simple_resource.for_each_empty
}

output "count_zero_list" {
  value = simple_resource.count_zero
}

output "for_each_empty_dynamic_map" {
  value = simple_resource.for_each_empty_dynamic
}

output "anchor_result" {
  value = simple_resource.anchor.result
}
