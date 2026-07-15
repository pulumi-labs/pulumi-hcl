resource "destroyorder_resource" "first" {
  name = "first"
}

# `second` references `first` ONLY through replace_triggered_by, with no
# body-input, count/for_each, or depends_on link. Terraform derives a
# dependency from the trigger reference, so `second` is destroyed before
# `first` and the two deletes never overlap.
resource "destroyorder_resource" "second" {
  name = "second"
  lifecycle {
    replace_triggered_by = [destroyorder_resource.first.id]
  }
}
