resource "simple_resource" "old" {
  input_one = "a"
}

output "r" {
  value = simple_resource.old.result
}
