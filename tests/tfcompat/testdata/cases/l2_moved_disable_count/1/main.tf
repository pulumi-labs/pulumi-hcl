resource "simple_resource" "a" {
  input_one = "x"
}

moved {
  from = simple_resource.a[0]
  to   = simple_resource.a
}

output "r" { value = simple_resource.a.result }
