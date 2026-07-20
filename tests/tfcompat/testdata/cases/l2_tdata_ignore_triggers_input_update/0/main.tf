# Stage 0: create. Only triggers_replace is ignored; input is not. A later input
# change must still update in place — suppressing the trigger must not suppress
# the input path. simple_resource.dependent witnesses terraform_data.t's id
# stability via replace_triggered_by.
resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = "x"
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
