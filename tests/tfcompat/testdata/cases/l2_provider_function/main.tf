terraform {
  required_providers {
    pfx = {
      source = "hashicorp/pfx"
    }
  }
}

resource "pfx_thing" "a" {
  name = provider::pfx::concat_str("hello-", "world")
}

# The second argument is another resource's computed id, so the call runs only
# once the value is known.
resource "pfx_thing" "b" {
  name = provider::pfx::concat_str("id-", pfx_thing.a.id)
}

output "plain" {
  value = provider::pfx::concat_str("a-", "b")
}

output "joined" {
  value = provider::pfx::join_str("-", "a", "b", "c")
}

output "joined_expanded" {
  value = provider::pfx::join_str(".", ["x", "y"]...)
}

output "joined_empty" {
  value = provider::pfx::join_str("-")
}

# A null argument to the null-allowing `second` parameter is deliberately not
# covered here: OpenTofu 1.12 skips marshaling null arguments (plugin6
# grpc_provider.go CallFunction), handing terraform-plugin-framework an empty
# DynamicValue it rejects, so the call fails under tofu for reasons unrelated
# to pulumi-hcl. Null-argument handling is covered by unit tests instead.
