resource "simple_resource" "a" {
  for_each  = toset(["x"])
  input_one = "x"
}

moved {
  from = simple_resource.a[0]
  to   = simple_resource.a["x"]
}

output "r" { value = simple_resource.a["x"].result }
