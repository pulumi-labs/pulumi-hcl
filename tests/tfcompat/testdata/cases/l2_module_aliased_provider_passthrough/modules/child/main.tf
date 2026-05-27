# `configuration_aliases = [simple.foo]` is the standard TF way for a
# child module to declare that it expects to be passed an aliased
# `simple.foo` provider via its caller's `providers = { ... }` block.
# tofu accepts the `simple.foo` traversal as the alias declaration;
# pulumi-hcl's parser rejects it with "Variables not allowed".
terraform {
  required_providers {
    simple = {
      configuration_aliases = [simple.foo]
    }
  }
}

resource "simple_resource" "r" {
  provider  = simple.foo
  input_one = "world"
  input_two = true
}

output "resource_prefix_result" {
  value = simple_resource.r.prefix_result
}
