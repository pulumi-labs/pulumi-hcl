terraform {
  required_providers {
    pfx = {
      source = "hashicorp/pfx"
    }
  }
}

output "broken" {
  value = provider::pfx::concat_str("boom", "x")
}
