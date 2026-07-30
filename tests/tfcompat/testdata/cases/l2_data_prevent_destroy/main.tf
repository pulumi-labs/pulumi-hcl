# Lifecycle arguments are defined only for managed resources; a data block's
# lifecycle carries only conditions.
data "simple_lookup" "d" {
  query = "x"

  lifecycle {
    prevent_destroy = true
  }
}
