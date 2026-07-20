# Stage 0: create. ignore_changes = all covers every attribute, so later changes
# to both input and triggers_replace must be suppressed. output mirrors input, so
# `value` witnesses the retained input; simple_resource.dependent witnesses
# terraform_data.t's id stability via replace_triggered_by.
resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = "x"
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
