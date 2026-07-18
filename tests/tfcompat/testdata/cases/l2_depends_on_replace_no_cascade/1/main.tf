# Stage 1: change `force` on pool[0] only (A -> B). `force` is ForceNew, so
# pool[0] is replaced; pool[1] is untouched.
#
# depends_on is ordering-only: replacing pool[0] must leave `dependent`
# alone, as its literal inputs cannot have changed.

resource "replacer_resource" "pool" {
  count = 2
  force = count.index == 0 ? "B" : "const"
}

resource "replacer_resource" "dependent" {
  force      = "d"
  note       = "n"
  depends_on = [replacer_resource.pool]
}

output "dep" {
  value = replacer_resource.dependent.result
}
