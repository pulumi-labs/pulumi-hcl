variable "parent" {
  type = string
}

resource "cascade_child" "top" {
  parent = var.parent

  lifecycle {
    create_before_destroy = true
  }
}

output "witness" {
  value = cascade_child.top.witness
}
