resource "first" "simple:index:Resource" {
  value = false
}

resource "second" "simple:index:Resource" {
  value = invoke("simple-invoke:index:secretInvoke", {
    value          = "hello"
    secretResponse = first.value
  }).secret
}

