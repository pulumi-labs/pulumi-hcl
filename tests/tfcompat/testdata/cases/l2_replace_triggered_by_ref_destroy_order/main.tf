resource "order_resource" "first" {
  name         = "first"
  delay_create = true
}

# `second` references `first` ONLY through replace_triggered_by, with no
# body-input, count/for_each, or depends_on link. Terraform derives a
# dependency from the trigger reference, so the recorded sequence is
# [create first, create second, delete second, delete first]. The op that
# must complete first in each phase is delayed, so a missing edge flips the
# recorded order deterministically.
resource "order_resource" "second" {
  name         = "second"
  delay_delete = true
  lifecycle {
    replace_triggered_by = [order_resource.first.id]
  }
}
