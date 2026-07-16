# `a` declares create_before_destroy and is replaced across the two stages by a
# ForceNew change to `parent`. `b` depends on `a` (through a's computed
# `witness`, which is a ForceNew input on b), so b is replaced too, but b does
# NOT declare create_before_destroy.
#
# OpenTofu propagates create_before_destroy only to a resource's DEPENDENCIES,
# never to its dependents. b is a dependent of a, so b keeps the default
# delete-before-create ordering: b's old instance is destroyed before its
# replacement is created. Only a is created-before-destroyed.
#
# CascadeProvider bumps a shared counter on every create and delete and records
# the counter at each child's create in `witness`, making the interleaving of
# creates and deletes observable as a value.
resource "cascade_child" "a" {
  parent = "L2"
  lifecycle {
    create_before_destroy = true
  }
}

resource "cascade_child" "b" {
  parent = cascade_child.a.witness
}

output "witness_a" { value = cascade_child.a.witness }
output "witness_b" { value = cascade_child.b.witness }
