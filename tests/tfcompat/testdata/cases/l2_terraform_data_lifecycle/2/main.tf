# Stage 2: triggers_replace changes "x" -> "y" (input stays "b"). This forces a
# replacement of terraform_data.t, so terraform_data.t.id changes and
# simple_resource.dependent is replaced (Delete+Create on the simple provider).
resource "terraform_data" "t" {
  input            = "b"
  triggers_replace = "y"
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
