resource "simple_resource" "kept" { input_one = "k" }
resource "simple_resource" "r" { input_one = "x" }
output "kept" { value = simple_resource.kept.result }
