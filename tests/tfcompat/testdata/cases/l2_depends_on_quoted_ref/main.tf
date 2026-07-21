# OpenTofu 0.11 and earlier required `depends_on` entries to be quoted strings.
# The form is deprecated but still accepted: `configs.shimTraversalInString`
# re-parses the string as a traversal and emits only a warning.

resource "simple_resource" "first" {
  input_one = "first"
  input_two = true
}

resource "simple_resource" "second" {
  input_one  = "second"
  input_two  = false
  depends_on = ["simple_resource.first"]
}

output "first_result" {
  value = simple_resource.first.result
}

output "second_result" {
  value = simple_resource.second.result
}
