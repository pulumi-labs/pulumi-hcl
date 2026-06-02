# A repeating TypeSet block. The bridge pluralizes the name to `filters`, but
# the TF source writes one singular `filter {}` block per element — the same
# shape real providers use (e.g. the `filter` block on the `aws_ami` data
# source). The engine must map the singular block name onto the plural Pulumi
# property; tofu accepts it and pulumi-hcl must too.
resource "blocky_thing" "t" {
  name = "set"

  filter {
    name   = "image-id"
    values = "ami-123"
  }
  filter {
    name   = "owner"
    values = "self"
  }
}

output "summary" {
  value = blocky_thing.t.summary
}
