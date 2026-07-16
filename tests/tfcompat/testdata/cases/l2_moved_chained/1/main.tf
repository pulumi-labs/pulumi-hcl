resource "simple_resource" "c" { input_one = "x" }

# Two moved blocks chain in one config: the object created as `a` was renamed to
# `b`, then `b` was renamed to `c`. OpenTofu follows the chain (a -> b -> c) and
# moves the existing object to `c` with no create/delete.
moved {
  from = simple_resource.a
  to   = simple_resource.b
}
moved {
  from = simple_resource.b
  to   = simple_resource.c
}

output "r" { value = simple_resource.c.result }
