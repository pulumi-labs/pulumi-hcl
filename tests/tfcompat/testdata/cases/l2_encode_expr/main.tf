terraform {
  required_providers {
    terraform = {
      source = "terraform.io/builtin/terraform"
    }
  }
}

data "simple_lookup" "d" {
  query = "q"
}

# The builtin `terraform` provider ships no installable plugin, so
# provider::terraform::encode_expr must be implemented internally by both
# runtimes. It renders any value as HCL expression source text.
output "list" {
  value = provider::terraform::encode_expr([1, 2])
}

output "string" {
  value = provider::terraform::encode_expr("hello")
}

output "object" {
  value = provider::terraform::encode_expr({
    name = "alice"
    tags = ["a", "b"]
    n    = 3
  })
}

output "nested" {
  value = provider::terraform::encode_expr([{ a = 1 }, { a = 2 }])
}

# A value flowing out of a data source read must encode identically too. It is
# unknown at plan time, so the call has to defer to apply rather than fail.
# (The argument must be *wholly* unknown: encode_expr rejects composites that
# mix known and unknown parts, so no literal may wrap the data reference.)
output "data_attr" {
  value = provider::terraform::encode_expr(data.simple_lookup.d.prefix_result)
}

# Sensitive marks pass through: the result is sensitive, and the encoded text
# is the unmarked content — both for a wholly sensitive argument and for a
# composite with a sensitive part.
output "sens" {
  value     = provider::terraform::encode_expr(sensitive("secret"))
  sensitive = true
}

output "sens_inner" {
  value = provider::terraform::encode_expr({
    pw    = sensitive("secret")
    plain = 1
  })
  sensitive = true
}

output "is_sensitive" {
  value = issensitive(provider::terraform::encode_expr(sensitive("secret")))
}
