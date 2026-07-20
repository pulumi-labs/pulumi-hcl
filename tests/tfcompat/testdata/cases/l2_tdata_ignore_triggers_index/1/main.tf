# Stage 1: both keys change (keep "1" -> "2", drop "a" -> "b"). The nested
# ignore_changes entry ignores the whole triggers_replace attribute, so even the
# un-named "keep" key is suppressed: no replacement, id stays stable, and
# simple_resource.dependent is NOT replaced.
resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = { keep = "2", drop = "b" }
  lifecycle {
    ignore_changes = [triggers_replace["drop"]]
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
