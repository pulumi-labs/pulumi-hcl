# Stage 1: change `force` on pool[0] only (A -> B). `force` is ForceNew, so
# pool[0] is replaced; pool[1] is untouched.
#
# `dependent` references `pool[1]`, which does not change, so its trigger must
# not fire.
#
# OpenTofu: the indexed reference is scoped to that single instance -- only a
# change to pool[1] fires the dependent. pool[0]'s replacement is ignored, so
# `dependent` is left alone.
#
# pulumi-hcl: the indexed instance reference is not scoped to the instance; the
# dependent reacts to the replacement of ANY instance of the counted `pool`
# resource. pool[0]'s replacement therefore spuriously replaces `dependent`.
# The provider-op traces differ (pulumi records an extra create+delete of
# `dependent`).

resource "replacer_resource" "pool" {
  count = 2
  force = count.index == 0 ? "B" : "const"
}

resource "replacer_resource" "dependent" {
  force = "d"
  note  = "n"
  lifecycle {
    replace_triggered_by = [replacer_resource.pool[1]]
  }
}

output "dep" {
  value = replacer_resource.dependent.result
}
