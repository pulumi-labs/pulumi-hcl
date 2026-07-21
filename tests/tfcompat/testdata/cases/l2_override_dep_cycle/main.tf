# `a` refers to `b` and `b` refers to `a`, but `override.tf` replaces the
# argument of `a` that holds the reference, so the merged configuration has
# no cycle. Overrides are applied when the configuration is loaded, before
# any dependency graph is built.
resource "simple_resource" "a" {
  input_one = simple_resource.b.result
}

resource "simple_resource" "b" {
  input_one = simple_resource.a.result
}

output "a_result" { value = simple_resource.a.result }
output "b_result" { value = simple_resource.b.result }
