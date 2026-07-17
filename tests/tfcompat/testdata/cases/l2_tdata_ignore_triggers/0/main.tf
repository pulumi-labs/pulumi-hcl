# triggers_replace changes are ignored via ignore_changes, so a later change to
# it must NOT force a replacement. simple_resource.dependent witnesses
# terraform_data.t.id stability through replace_triggered_by.
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
