terraform {
  required_providers {
    pfx = {
      source = "hashicorp/pfx"
    }
  }
}

resource "pfx_thing" "upstream" {
  name = "alpha"
}

resource "pfx_thing" "downstream" {
  name = provider::pfx::concat_str("id-", pfx_thing.upstream.id)
}
