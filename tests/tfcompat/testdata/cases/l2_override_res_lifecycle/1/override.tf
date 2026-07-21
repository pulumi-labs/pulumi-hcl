# The override's lifecycle block sets only create_before_destroy, so the
# ignore_changes list declared in main.tf stays in effect.
resource "simple_resource" "r" {
  lifecycle {
    create_before_destroy = true
  }
}
