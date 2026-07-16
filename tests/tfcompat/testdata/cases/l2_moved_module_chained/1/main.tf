module "c" { source = "./mod" }

# Two moved blocks chain in one config: the call created as `module.a` was
# renamed to `module.b`, then `module.b` was renamed to `module.c`. OpenTofu
# follows the chain and moves the existing objects to `module.c` with no
# create/delete.
moved {
  from = module.a
  to   = module.b
}
moved {
  from = module.b
  to   = module.c
}

output "r" { value = module.c.r }
