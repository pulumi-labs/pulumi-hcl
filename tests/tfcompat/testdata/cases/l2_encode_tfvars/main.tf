terraform {
  required_providers {
    terraform = {
      source = "terraform.io/builtin/terraform"
    }
  }
}

resource "terraform_data" "obj" {
  input = {
    a = 1
    b = "two"
  }
}

# The builtin `terraform` provider ships no installable plugin, so
# provider::terraform::encode_tfvars must be implemented internally by both
# runtimes. It renders an object as .tfvars file text, keys sorted, with a
# trailing newline.
output "basic" {
  value = provider::terraform::encode_tfvars({ foo = "bar", baz = 1 })
}

output "nested" {
  value = provider::terraform::encode_tfvars({
    list = [1, "two", true]
    obj  = { a = 1 }
  })
}

output "empty" {
  value = provider::terraform::encode_tfvars({})
}

# An object flowing out of a resource must encode identically too. It is
# wholly unknown at plan time, so the call has to defer to apply rather than
# fail. (Only *wholly* unknown arguments defer: an object literal wrapping the
# unknown reference is a known object with unknown parts, which errors.)
output "unknown_at_plan" {
  value = provider::terraform::encode_tfvars(terraform_data.obj.output)
}

# Sensitive marks pass through: the result is sensitive, and the encoded text
# is the unmarked content.
output "sens" {
  value     = provider::terraform::encode_tfvars({ pw = sensitive("secret") })
  sensitive = true
}

output "is_sensitive" {
  value = issensitive(provider::terraform::encode_tfvars({ pw = sensitive("secret") }))
}
