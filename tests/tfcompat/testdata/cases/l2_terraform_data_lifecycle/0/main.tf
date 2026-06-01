# Stage 0: create. terraform_data.t holds input="a" with triggers_replace="x".
# simple_resource.dependent is replaced whenever terraform_data.t.id changes, so
# its provider-op trace witnesses terraform_data's id lifecycle across stages
# (the id itself is a random uuid and can't be compared cross-path directly).
resource "terraform_data" "t" {
  input            = "a"
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
