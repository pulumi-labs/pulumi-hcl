# A repeating TypeSet block on a data source. The bridge pluralizes the name
# (`filter` -> `filters`), but the TF source writes one singular `filter {}`
# block per element — the exact shape of the `filter` block on the `aws_ami`
# data source used by RaJiska/fck-nat. The engine must map the singular block
# name onto the plural Pulumi property on the data-source path too.
data "blocky_image" "img" {
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
  value = data.blocky_image.img.summary
}
