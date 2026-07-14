terraform {
  required_providers {
    pfx = {
      source = "hashicorp/pfx"
    }
  }
}

# Call concat_str with only its required first argument, omitting the
# null-allowing `second` parameter entirely.
output "partial" {
  value = provider::pfx::concat_str("only-first")
}
