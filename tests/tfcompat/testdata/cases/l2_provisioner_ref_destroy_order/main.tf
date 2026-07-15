resource "order_resource" "a" {
  name         = "a"
  delay_create = true
}

# `b` references `a` ONLY through a provisioner's command, with no body-input,
# count/for_each, depends_on, or lifecycle link. Terraform derives a dependency
# from the provisioner reference, so the recorded sequence is [create a, create
# b, delete b, delete a]. A runtime that collects dependencies only from a
# resource's body/meta-arguments and skips provisioner blocks will miss the
# edge; the op that must complete first in each phase is delayed, so a missing
# edge flips the recorded order deterministically.
resource "order_resource" "b" {
  name         = "b"
  delay_delete = true

  provisioner "local-exec" {
    command = "echo ${order_resource.a.result}"
  }
}

output "a_result" {
  value = order_resource.a.result
}
