# Stage 1: input "a" -> "b" (not ignored) and triggers_replace "x" -> "y"
# (ignored). The trigger change is suppressed so there is no replacement — id
# stays stable and simple_resource.dependent is not touched — but the input
# update still applies in place, so output follows the new input ("b").
resource "terraform_data" "t" {
  input            = "b"
  triggers_replace = "y"
  lifecycle {
    ignore_changes = [triggers_replace]
  }
}

resource "simple_resource" "dependent" {
  input_one = "constant"
  lifecycle {
    replace_triggered_by = [terraform_data.t.id]
  }
}

output "value" { value = terraform_data.t.output }
output "dependent_result" { value = simple_resource.dependent.result }
