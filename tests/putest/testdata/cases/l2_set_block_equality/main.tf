# `filter` is a repeating TypeSet nested block on blocky_thing. Through the
# dynamic bridge pulumi-hcl materializes it as an ordered tuple, so comparing
# it against a `toset(...)` of the same elements is a tuple-vs-set type
# mismatch and yields false, whatever the element order. (OpenTofu
# materializes a cty set and yields true for both — see the skipped tfcompat
# case of the same name.)
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
