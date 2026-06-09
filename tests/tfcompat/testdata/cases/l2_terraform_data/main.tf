terraform {
  required_version = ">= 1.0"
}

resource "terraform_data" "example" {
  input = "hello"
}

# input is optional in Terraform: a terraform_data used only for its
# triggers_replace lifecycle omits it, leaving input/output null.
resource "terraform_data" "no_input" {
  triggers_replace = ["v1"]
}

# input/output are dynamically typed, so a heterogeneous tuple must round-trip
# with each element's type intact rather than being coerced to a single element
# type. An object with mixed-typed fields keeps its per-attribute types too.
resource "terraform_data" "tuple" {
  input = [1, "two", true]
}

resource "terraform_data" "obj" {
  input = {
    name = "alice"
    age  = 30
    tags = ["a", "b"]
  }
}

# An object whose attributes are all scalars of differing types must keep each
# attribute's type rather than collapsing to a single-typed map.
resource "terraform_data" "scalar_obj" {
  input = {
    a = 1
    b = "two"
    c = true
  }
}

output "value" {
  value = terraform_data.example.output
}

output "input_echo" {
  value = terraform_data.example.input
}

output "no_input_output_null" {
  value = terraform_data.no_input.output == null
}

output "no_input_input_null" {
  value = terraform_data.no_input.input == null
}

output "no_input_triggers" {
  value = terraform_data.no_input.triggers_replace
}

output "tuple_output" {
  value = terraform_data.tuple.output
}

output "obj_output" {
  value = terraform_data.obj.output
}

output "scalar_obj_output" {
  value = terraform_data.scalar_obj.output
}
