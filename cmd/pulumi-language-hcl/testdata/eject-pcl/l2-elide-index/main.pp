resource "res" "simple:index:Resource" {
  value = true
}

output "inv" {
  value = invoke("simple-invoke:index:myInvoke", {
    value = "test"
  }).result
}

