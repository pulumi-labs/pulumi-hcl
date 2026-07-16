resource "simple_resource" "a" {
  count     = 1
  input_one = "x"
}
output "r" { value = simple_resource.a[0].result }
