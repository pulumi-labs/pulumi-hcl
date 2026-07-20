# Stage 1: driver.result changes (input_one "p" -> "q") and triggers_replace
# changes "x" -> "y". triggers_replace is ignored, but the replace_triggered_by
# on driver.result still fires, so terraform_data.t is replaced, its id rolls,
# and simple_resource.dependent is replaced (Delete+Create on the simple
# provider).
resource "simple_resource" "driver" {
  input_one = "q"
}

resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = "y"
  lifecycle {
    ignore_changes       = [triggers_replace]
    replace_triggered_by = [simple_resource.driver.result]
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
