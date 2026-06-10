resource "simple_resource" "a" {
  for_each  = toset(["small"])
  input_one = "fixed"
}
output "r" { value = simple_resource.a["small"].result }
