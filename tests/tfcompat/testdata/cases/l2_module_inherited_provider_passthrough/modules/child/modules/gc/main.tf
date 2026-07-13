resource "simple_resource" "r" {
  input_one = "x"
  input_two = true
}

# prefix_result folds in the provider's configured `prefix`, so the output also
# proves the *inherited* config (prefix = "root-prefix") reached this resource.
output "result" {
  value = simple_resource.r.prefix_result
}
