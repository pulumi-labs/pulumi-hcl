# Submodule declares its own `provider "simple"` with config that would
# fail ConfigureContextFunc. No resource in this module uses it (the
# resource below uses the inherited default provider from the parent),
# so Terraform/tofu never configures it.
provider "simple" {
  fail_configure = true
}

resource "simple_resource" "in_module" {
  input_one = "from-module"
  input_two = false
}

output "result" {
  value = simple_resource.in_module.prefix_result
}
