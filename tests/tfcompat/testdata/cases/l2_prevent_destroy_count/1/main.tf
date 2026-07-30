# Shrinking count orphans instance [1], but the block still carries the
# guard, so the plan is refused.
resource "simple_resource" "guarded" {
  count     = 1
  input_one = "n-${count.index}"

  lifecycle {
    prevent_destroy = true
  }
}
