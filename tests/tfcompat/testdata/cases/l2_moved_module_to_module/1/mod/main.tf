resource "simple_resource" "r" { input_one = "x" }
output "r" { value = simple_resource.r.result }
