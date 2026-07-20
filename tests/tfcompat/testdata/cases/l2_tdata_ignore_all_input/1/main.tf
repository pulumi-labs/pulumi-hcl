# Stage 1: input "a" -> "b" and triggers_replace "x" -> "y". ignore_changes = all
# suppresses both: no in-place update and no replacement. output stays "a" (the
# retained input), id stays stable, and simple_resource.dependent is not touched.
resource "terraform_data" "t" {
  input            = "b"
  triggers_replace = "y"
  lifecycle {
    ignore_changes = all
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
