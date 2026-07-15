# Stage 1: triggers_replace is removed entirely ("x" -> null). OpenTofu treats
# clearing this ForceNew attribute as a change, so terraform_data.t is replaced,
# its id rolls, and simple_resource.dependent is replaced (Delete+Create on the
# simple provider). Input is unchanged.
resource "terraform_data" "t" {
  input = "constant"
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
