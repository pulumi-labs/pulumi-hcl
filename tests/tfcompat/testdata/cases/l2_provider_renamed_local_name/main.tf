# The provider's local name ("myp") differs from its package name ("simple",
# the basename of the source). OpenTofu resolves the local name to the source
# and runs the program. The resource is tied to the "myp" configuration via
# the `provider` meta-argument.
terraform {
  required_providers {
    myp = {
      source = "hashicorp/simple"
    }
  }
}

provider "myp" {
  prefix = "hello"
}

resource "simple_resource" "r" {
  provider  = myp
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
