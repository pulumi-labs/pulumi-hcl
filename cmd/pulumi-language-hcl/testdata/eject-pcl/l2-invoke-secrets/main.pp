resource "res" "simple:index:Resource" {
  value = true
}

output "nonSecret" {
  value = invoke("simple-invoke:index:secretInvoke", {
    value          = "hello"
    secretResponse = false
  }).response
}

output "firstSecret" {
  value = invoke("simple-invoke:index:secretInvoke", {
    value          = "hello"
    secretResponse = res.value
  }).response
}

output "secondSecret" {
  value = invoke("simple-invoke:index:secretInvoke", {
    value          = secret("goodbye")
    secretResponse = false
  }).response
}

