resource "orderdep_resource" "a" {
  name = "a"
}

# `b` references `a` only through its `count` meta-argument. TF records this as
# a dependency, so `b` is destroyed before `a`. The reference never appears in
# `b`'s body, so a runtime that only derives dependencies from input values
# will miss the edge and destroy `a` before `b`.
resource "orderdep_resource" "b" {
  count = orderdep_resource.a.name == "a" ? 1 : 0
  name  = "b"
  needs = "a"
}

output "a_result" {
  value = orderdep_resource.a.result
}
