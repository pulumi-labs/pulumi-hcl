# `dependent` orders after the counted `pool` resource through depends_on
# only; its body is all literals. Stage 0 stands up both pool instances and
# the dependent.

resource "replacer_resource" "pool" {
  count = 2
  force = count.index == 0 ? "A" : "const"
}

resource "replacer_resource" "dependent" {
  force      = "d"
  note       = "n"
  depends_on = [replacer_resource.pool]
}

output "dep" {
  value = replacer_resource.dependent.result
}
