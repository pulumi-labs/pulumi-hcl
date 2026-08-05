# `filter` is a repeating TypeSet nested block on blocky_thing. It
# materializes as a cty set, so comparing it against a `toset(...)` of the
# same elements is true regardless of element order, matching OpenTofu. The
# terraform-provider plugin path materializes an ordered tuple and yields
# false — the divergence the tfcompat case of the same name is skipped for.
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

output "eq_same" {
  value = blocky_thing.t.filter == toset([
    { name = "zebra", values = "z" },
    { name = "apple", values = "a" },
  ])
}

output "eq_reordered" {
  value = blocky_thing.t.filter == toset([
    { name = "apple", values = "a" },
    { name = "zebra", values = "z" },
  ])
}
