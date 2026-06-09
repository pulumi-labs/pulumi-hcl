resource "simple_resource" "new" {
  input_one = "m"
}
moved {
  from = simple_resource.old
  to   = simple_resource.new
}
output "r" { value = simple_resource.new.result }
