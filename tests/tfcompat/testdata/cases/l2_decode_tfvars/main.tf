terraform {
  required_providers {
    terraform = {
      source = "terraform.io/builtin/terraform"
    }
  }
}

resource "terraform_data" "content" {
  input = "x = 1\ny = \"two\"\n"
}

# The builtin `terraform` provider ships no installable plugin, so
# provider::terraform::decode_tfvars must be implemented internally by both
# runtimes. It parses .tfvars text into an object.
output "basic" {
  value = provider::terraform::decode_tfvars("foo = \"bar\"\nbaz = 1\n")
}

output "nested" {
  value = provider::terraform::decode_tfvars("list = [1, \"two\", true]\nobj = {\n  a = 1\n}\n")
}

output "empty" {
  value = provider::terraform::decode_tfvars("")
}

# Constant expressions evaluate; only variables and function calls are
# rejected (the content is evaluated with no eval context).
output "exprs" {
  value = provider::terraform::decode_tfvars("sum = 1 + 2")
}

output "roundtrip" {
  value = provider::terraform::decode_tfvars(provider::terraform::encode_tfvars({ a = 1, b = "x" }))
}

# Content flowing out of a resource is unknown at plan time, so the call has
# to defer to apply rather than fail.
output "unknown_at_plan" {
  value = provider::terraform::decode_tfvars(terraform_data.content.output)
}

# A sensitive input marks the whole decoded object sensitive.
output "sens" {
  value     = provider::terraform::decode_tfvars(sensitive("a = 1"))
  sensitive = true
}

output "is_sensitive" {
  value = issensitive(provider::terraform::decode_tfvars(sensitive("a = 1")))
}
