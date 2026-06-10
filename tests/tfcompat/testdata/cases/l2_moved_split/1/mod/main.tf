resource "simple_resource" "a" { input_one = "x" }
output "r" { value = simple_resource.a.result }
