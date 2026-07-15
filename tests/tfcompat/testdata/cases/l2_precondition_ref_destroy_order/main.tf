resource "destroyorder_resource" "first" {
  name = "first"
}

# `second` references `first` ONLY through a precondition condition, with no
# body-input, count/for_each, or depends_on link. Terraform derives a
# dependency from the precondition reference, so `second` is destroyed before
# `first` and the two deletes never overlap.
resource "destroyorder_resource" "second" {
  name = "second"
  lifecycle {
    precondition {
      condition     = destroyorder_resource.first.id != ""
      error_message = "first must exist"
    }
  }
}
