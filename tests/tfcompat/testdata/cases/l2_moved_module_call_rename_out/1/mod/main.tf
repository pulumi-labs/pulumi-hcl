resource "simple_resource" "kept" { input_one = "k" }
output "kept" { value = simple_resource.kept.result }
