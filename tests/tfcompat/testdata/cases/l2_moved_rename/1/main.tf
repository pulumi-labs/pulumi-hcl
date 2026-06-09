resource "simple_resource" "new" {
  input_one = "a"
}

moved {
  from = simple_resource.old
  to   = simple_resource.new
}

output "r" {
  value = simple_resource.new.result
}
