# Stage 1: input changes "a" -> "b" while triggers_replace stays "x". This is an
# in-place update: terraform_data.t.id is stable, so simple_resource.dependent is
# NOT replaced. output follows the new input.
resource "terraform_data" "t" {
  input            = "b"
  triggers_replace = "x"
}

resource "simple_resource" "dependent" {
  input_one = "constant"
  lifecycle {
    replace_triggered_by = [terraform_data.t.id]
  }
}

output "value" {
  value = terraform_data.t.output
}

output "dependent_result" {
  value = simple_resource.dependent.result
}

output "input_echo" {
  value = terraform_data.t.input
}

output "triggers_echo" {
  value = terraform_data.t.triggers_replace
}
