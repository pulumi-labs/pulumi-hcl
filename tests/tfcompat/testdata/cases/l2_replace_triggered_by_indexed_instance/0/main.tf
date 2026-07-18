# `dependent` references a SINGLE indexed instance of the counted `pool`
# resource (`pool[1]`) in replace_triggered_by. Stage 0 stands up two `pool`
# instances plus the dependent. `pool[1]` is the instance the dependent tracks;
# `pool[0]` is a sibling it must ignore.

resource "replacer_resource" "pool" {
  count = 2
  force = count.index == 0 ? "A" : "const"
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
