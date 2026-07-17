# `b`'s count references a single for_each instance. Count must be known at
# plan time, so it references the config-known `name` (a computed attribute
# would make tofu fail with "Invalid count argument") — the reference still
# creates a dependency edge. OpenTofu resolves the literal-indexed
# meta-argument reference to the exact instance: `b` creates after `a["x"]`
# only, not after the delayed `a["y"]`, so the recorded create order is
# [a["x"], b, a["y"]]. A runtime that widens meta-argument references to the
# whole resource makes `b` wait for `a["y"]` too — a deterministic flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

resource "order_resource" "b" {
  count = order_resource.a["x"].name == "x" ? 1 : 0
  name  = "b"
}

output "b_result" {
  value = order_resource.b[0].result
}
