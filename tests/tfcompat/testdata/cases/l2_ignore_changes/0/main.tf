resource "simple_resource" "r" {
  input_one = "a"
  input_two = false

  lifecycle {
    ignore_changes = [input_one]
  }
}

resource "simple_resource" "all" {
  input_one = "x"
  input_two = false

  lifecycle {
    ignore_changes = all
  }
}

output "result" {
  value = simple_resource.r.result
}

output "input_one" {
  value = simple_resource.r.input_one
}

output "all_input_one" {
  value = simple_resource.all.input_one
}

output "all_result" {
  value = simple_resource.all.result
}
