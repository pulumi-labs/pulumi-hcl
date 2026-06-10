resource "simple_resource" "a" {
  count     = 1
  input_one = "x"
}
moved {
  from = simple_resource.a
  to   = simple_resource.a[0]
}
output "r" { value = simple_resource.a[0].result }
