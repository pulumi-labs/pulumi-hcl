# Stage 0: create. triggers_replace is a map, and ignore_changes names a single
# key (triggers_replace["drop"]). triggers_replace is a single dynamic attribute,
# so an ignore_changes traversal into it ignores the whole attribute — the same
# way it does for input. simple_resource.dependent witnesses whether
# terraform_data.t is replaced across stages via replace_triggered_by.
resource "terraform_data" "t" {
  input            = "a"
  triggers_replace = { keep = "1", drop = "a" }
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
