provider "simple" {
  prefix = "mid-prefix"
}

module "grandchild" {
  source = "./modules/grandchild"
}
