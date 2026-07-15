# The create_before_destroy resource lives inside a child module and depends on
# a root resource through a module input. Changing base's ForceNew label
# replaces base and (through the ForceNew module input) the in-module child, so
# create_before_destroy must propagate across the module boundary to base. When
# it does, every create runs before any delete and the child records witness = 2.
resource "cascade_parent" "base" {
  label = "L2"
}

module "mod" {
  source = "./modules/child"
  parent = cascade_parent.base.result
}

output "witness" {
  value = module.mod.witness
}
