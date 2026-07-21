resource "simple_resource" "old" { input_one = "m" }
output "r" { value = simple_resource.old.result }
