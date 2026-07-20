# Stage 0: create. terraform_data.t ignores triggers_replace but ALSO carries a
# replace_triggered_by on simple_resource.driver.result. The ignored trigger must
# not clobber the working one: when driver.result changes, t must still be
# replaced. simple_resource.dependent witnesses that replacement.
resource "simple_resource" "driver" {
  input_one = "p"
}

resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = "x"
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
