resource "cascade_parent" "p" {
  label = "L1"
}

resource "cascade_child" "c" {
  parent = cascade_parent.p.result

  lifecycle {
    create_before_destroy = true
  }
}

output "witness" {
  value = cascade_child.c.witness
}
