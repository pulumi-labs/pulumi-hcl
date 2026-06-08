provider "simple" {
  prefix = "root-prefix"
}

# P2: child with no provider block inherits the root default config.
module "p2_plain" {
  source = "./modules/p2_plain"
}

# P3: child with a `required_providers` block (but no config block) still
# inherits the root default config.
module "p3_required_providers" {
  source = "./modules/p3_required_providers"
}

# P4: inheritance is recursive — a grandchild two levels deep inherits the
# root default config.
module "p4_grandchild" {
  source = "./modules/p4_grandchild"
}

# P6: a data source (not a resource) in a child inherits the root default
# config.
module "p6_data_source" {
  source = "./modules/p6_data_source"
}

