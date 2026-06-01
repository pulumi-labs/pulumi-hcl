resource "simple_resource" "trigger" {
  input_one = "b"
}

resource "simple_resource" "dependent" {
  input_one = "constant"
  lifecycle {
    replace_triggered_by = [simple_resource.trigger.result]
  }
}

output "trigger_result" {
  value = simple_resource.trigger.result
}

output "dependent_result" {
  value = simple_resource.dependent.result
}
