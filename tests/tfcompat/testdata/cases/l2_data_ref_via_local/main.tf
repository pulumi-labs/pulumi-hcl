# A data source that references a resource only through a root local. The
# read cannot happen at plan time — its input is computed by the resource —
# so it defers until the resource is applied.
resource "simple_resource" "res" {
  input_one = "a"
}

locals {
  val = simple_resource.res.result
}

data "simple_lookup" "lookup" {
  query = local.val
}

output "out" {
  value = data.simple_lookup.lookup.prefix_result
}
