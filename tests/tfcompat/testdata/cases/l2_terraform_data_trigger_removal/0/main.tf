# Stage 0: create. terraform_data.t has triggers_replace = "x".
# simple_resource.dependent watches terraform_data.t.id via replace_triggered_by,
# so the simple provider's op trace witnesses whether terraform_data is replaced
# across stages (its id is a random uuid, not comparable cross-path directly).
resource "terraform_data" "t" {
  input            = "constant"
  triggers_replace = "x"
}

resource "simple_resource" "dependent" {
  input_one = "constant"
  lifecycle {
    replace_triggered_by = [terraform_data.t.id]
  }
}

output "dependent_result" {
  value = simple_resource.dependent.result
}
