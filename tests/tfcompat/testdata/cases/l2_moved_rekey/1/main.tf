resource "simple_resource" "a" {
  for_each  = toset(["tiny"])
  input_one = "fixed"
}
moved {
  from = simple_resource.a["small"]
  to   = simple_resource.a["tiny"]
}
output "r" { value = simple_resource.a["tiny"].result }
