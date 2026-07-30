# The guarded resource is gone from configuration. prevent_destroy is
# config-gated: removing the block removes the guard with it, so this single
# apply destroys the orphaned instance and succeeds.
output "done" {
  value = "done"
}
