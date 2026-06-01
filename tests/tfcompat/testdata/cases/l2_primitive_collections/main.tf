# Every collection shape in one case. The `all` resource carries a `tags` map,
# a `ports` set, and a `zones` list (primitive collections), a `rule` TypeSet of
# blocks whose elements are heterogeneously typed (one omits the optional
# `protocol`), and a `metadata` block whose own attributes are a map and a set
# (collections nested inside a block). The `nulls` resource sets the primitive
# collections to null to confirm null is treated as unset, not as an empty
# collection. Both paths must feed the provider identical values regardless of
# block/element order.
resource "collections_thing" "all" {
  tags = {
    env  = "prod"
    team = "platform"
  }
  ports = [443, 80, 8080]
  zones = ["us-east-1a", "us-east-1b"]

  rule {
    port     = 443
    protocol = "tcp"
  }
  rule {
    port = 80
  }
  rule {
    port     = 53
    protocol = "udp"
  }

  metadata {
    labels = {
      team = "platform"
      env  = "prod"
    }
    selectors = ["az-b", "az-a"]
  }
}

resource "collections_thing" "nulls" {
  tags  = null
  ports = null
  zones = ["anchor"]
}

output "all_summary" {
  value = collections_thing.all.summary
}

output "null_summary" {
  value = collections_thing.nulls.summary
}
