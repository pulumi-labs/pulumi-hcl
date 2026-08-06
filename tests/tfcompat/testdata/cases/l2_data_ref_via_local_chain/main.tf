# A data source that reaches a resource through a chain of root locals. The
# read defers until the resource is applied, exactly as with a single local.
resource "simple_resource" "res" {
  input_one = "a"
}

locals {
  val     = simple_resource.res.result
  chained = "${local.val}-c"
}

data "simple_lookup" "lookup" {
  query = local.chained
}

output "out" {
  value = data.simple_lookup.lookup.prefix_result
}
