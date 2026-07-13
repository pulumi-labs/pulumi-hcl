# module.consumer depends_on module.producer with no data reference between
# them, so only the explicit depends_on can order the two ordering_resource
# creates. Each create fails if another create is in flight, so a violation of
# the depends_on ordering surfaces as overlapping creates. Terraform serializes
# the modules and both creates succeed.
module "producer" {
  source = "./producer"
}

module "consumer" {
  source     = "./consumer"
  depends_on = [module.producer]
}
