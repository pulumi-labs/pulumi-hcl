# `filter` is a repeating TypeSet nested block on blocky_thing. In OpenTofu a
# set block materializes as a cty *set*, whose equality is content-based and
# order-independent. pulumi-hcl materializes it as an ordered tuple, so
# comparing it against a `toset(...)` of the same elements diverges.
resource "blocky_thing" "t" {
  name = "seteq"
  filter {
    name   = "zebra"
    values = "z"
  }
  filter {
    name   = "apple"
    values = "a"
  }
}

# Same elements in the same written order. A set equals the set of its
# elements, so OpenTofu yields true; pulumi-hcl compares a tuple to a set and
# yields false.
output "eq_same" {
  value = blocky_thing.t.filter == toset([
    { name = "zebra", values = "z" },
    { name = "apple", values = "a" },
  ])
}

# Same elements, reordered literal. Set equality ignores order, so OpenTofu
# still yields true; pulumi-hcl still yields false.
output "eq_reordered" {
  value = blocky_thing.t.filter == toset([
    { name = "apple", values = "a" },
    { name = "zebra", values = "z" },
  ])
}
