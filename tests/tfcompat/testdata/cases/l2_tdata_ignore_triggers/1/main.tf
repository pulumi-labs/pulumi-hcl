# Stage 1: triggers_replace changes "x" -> "y" but is ignored. No replacement of
# terraform_data.t; id stays stable; dependent is NOT replaced.
resource "terraform_data" "t" {
  input            = "a"
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
