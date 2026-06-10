provider "pfx" {
  prefix = "pre"
}

resource "pfx_thing" "t" {
  name = "alpha"
}

data "pfx_lookup" "l" {
  query = pfx_thing.t.name
}

output "echo" {
  value = pfx_thing.t.echo
}

output "prefix_result" {
  value = pfx_thing.t.prefix_result
}

output "lookup" {
  value = data.pfx_lookup.l.prefix_result
}
