resource "simple_resource" "res" { input_one = "m" }
output "r" { value = simple_resource.res.result }
