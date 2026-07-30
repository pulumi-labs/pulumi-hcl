# prevent_destroy may not refer to per-instance symbols: the guard must be
# evaluable for instances that have already been removed from configuration.
resource "simple_resource" "guarded" {
  count     = 2
  input_one = "n-${count.index}"

  lifecycle {
    prevent_destroy = count.index == 0
  }
}
